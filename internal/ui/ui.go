// Package ui is the Bubble Tea application: a map on the left, a list of quakes on the right.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/torihazi/yure/internal/japanmap"
	"github.com/torihazi/yure/internal/quake"
)

const (
	panelWidth   = 38
	rippleLength = 3 * time.Second
	frameRate    = 80 * time.Millisecond
	demoInterval = 4 * time.Second
	demoReports  = 20
)

// Options are set from command-line flags.
type Options struct {
	// Demo replays recent real reports instead of waiting for new ones.
	Demo bool
}

type model struct {
	opts   Options
	src    *quake.Source
	events chan quake.Event

	all     []quake.Quake // everything received, newest first
	visible []quake.Quake // all, after the filter
	sel     int           // index into visible
	filter  filter

	loading bool // the initial history is still being fetched

	width, height int
	worldMap      *japanmap.Map

	rippleStart time.Time // when the current ripple animation began
	animating   bool      // whether a frame tick is scheduled
	status      string
	err         error
}

// Messages. Bubble Tea delivers these to Update.
type (
	loadedMsg struct {
		quakes []quake.Quake
		err    error
	}
	eventMsg quake.Event
	frameMsg struct{}
)

// Run starts the app and blocks until the user quits.
func Run(opts Options) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := model{
		opts:    opts,
		src:     quake.NewSource(),
		events:  make(chan quake.Event),
		status:  "connecting...",
		filter:  filter{period: defaultPeriod},
		loading: !opts.Demo,
	}
	// The feed is watched in its own goroutine, which hands events to the UI through the channel.
	if opts.Demo {
		go m.src.Replay(ctx, demoReports, demoInterval, m.events)
	} else {
		// Reports issued from now on come from Watch; earlier ones from Load in Init.
		go m.src.Watch(ctx, time.Now(), m.events)
	}

	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func (m model) Init() tea.Cmd {
	if m.opts.Demo {
		// The replay itself delivers the history, one report at a time.
		return waitForEvent(m.events)
	}
	return tea.Batch(load(m.src), waitForEvent(m.events))
}

// load fetches the longest period up front, so switching periods only filters.
func load(src *quake.Source) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		qs, err := src.Load(ctx, time.Now().Add(-periods[len(periods)-1].span))
		return loadedMsg{qs, err}
	}
}

// waitForEvent blocks until the stream sends something.
// Update calls it again after each event, so the UI keeps listening.
func waitForEvent(ch <-chan quake.Event) tea.Cmd {
	return func() tea.Msg { return eventMsg(<-ch) }
}

func frame() tea.Cmd {
	return tea.Tick(frameRate, func(time.Time) tea.Msg { return frameMsg{} })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.worldMap = japanmap.New(m.mapSize())
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.sel > 0 {
				m.sel--
				return m.startRipple()
			}
		case "down", "j":
			if m.sel < len(m.visible)-1 {
				m.sel++
				return m.startRipple()
			}
		case "enter", " ":
			return m.startRipple()
		case "p":
			m.filter.period = (m.filter.period + 1) % len(periods)
			m.refilter()
		case "s":
			m.filter.minScale = (m.filter.minScale + 1) % len(minScales)
			m.refilter()
		}

	case loadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		for _, q := range msg.quakes {
			m.all = quake.Merge(m.all, q)
		}
		m.refilter()
		return m.startRipple()

	case eventMsg:
		next := waitForEvent(m.events)
		switch {
		case msg.Err != nil && m.opts.Demo:
			m.err = msg.Err // the replay doesn't retry
		case msg.Err != nil:
			m.status = "feed error, retrying"
		case msg.OK && m.opts.Demo:
			m.status = "demo"
		case msg.OK:
			m.status = "live"
		case msg.Quake != nil:
			m.all = quake.Merge(m.all, *msg.Quake)
			m.refilter()
			i := indexOf(m.all, msg.Quake.ID)
			if i < 0 {
				return m, next // a cancellation removed it
			}
			// Jump to the quake so it's impossible to miss, unless the filter hides it.
			// It's usually at the top, but a late report can update an older entry.
			if i := indexOf(m.visible, msg.Quake.ID); i >= 0 {
				m.sel = i
				var ripple tea.Cmd
				m, ripple = m.startRipple()
				return m, tea.Batch(next, ripple)
			}
			return m, next
		}
		return m, next

	case frameMsg:
		if time.Since(m.rippleStart) < rippleLength {
			return m, frame()
		}
		m.animating = false
	}
	return m, nil
}

// indexOf returns the index of the quake with the given ID, or -1.
func indexOf(quakes []quake.Quake, id string) int {
	for i, q := range quakes {
		if q.ID == id {
			return i
		}
	}
	return -1
}

// refilter recomputes visible, keeping the selected quake selected if it's still shown.
func (m *model) refilter() {
	var selected string
	if m.sel < len(m.visible) {
		selected = m.visible[m.sel].ID
	}
	m.visible = m.filter.apply(m.all, time.Now())
	m.sel = max(0, indexOf(m.visible, selected))
}

// startRipple restarts the ripple, scheduling frames only if none are pending.
// Scheduling a second tick chain would make the animation run at double speed.
func (m model) startRipple() (model, tea.Cmd) {
	m.rippleStart = time.Now()
	if m.animating {
		return m, nil
	}
	m.animating = true
	return m, frame()
}

func (m model) mapSize() (cols, rows int) {
	return max(20, m.width-panelWidth-4), max(10, m.height-2)
}

// JMA's standard colors for each intensity.
var scaleColors = map[quake.Scale]lipgloss.Color{
	10: "#F2F2FF", 20: "#00AAFF", 30: "#0041FF", 40: "#FAE696",
	45: "#FFE600", 46: "#FFE600", 50: "#FF9900", 55: "#FF2800", 60: "#A50021", 70: "#B40068",
}

func scaleStyle(s quake.Scale) lipgloss.Style {
	c, ok := scaleColors[s]
	if !ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	}
	fg := lipgloss.Color("#000000")
	if s == 30 || s >= 55 {
		fg = "#FFFFFF" // dark backgrounds
	}
	return lipgloss.NewStyle().Background(c).Foreground(fg).Bold(true)
}

var (
	coastStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#5A6A7A"))
	centerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3030")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	titleStyle  = lipgloss.NewStyle().Bold(true)
	selStyle    = lipgloss.NewStyle().Background(lipgloss.Color("#303848"))
)

func (m model) View() string {
	if m.worldMap == nil {
		return "loading..."
	}
	left := m.viewMap()
	right := m.viewPanel()
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "  ", right)
	return body + "\n" + m.viewFooter()
}

func (m model) viewMap() string {
	var rings []japanmap.Ring
	var marks []japanmap.Mark
	if m.sel < len(m.visible) {
		q := m.visible[m.sel]
		for pref, s := range q.PrefScales {
			if pos, ok := japanmap.Prefectures[pref]; ok {
				marks = append(marks, japanmap.Mark{Lon: pos[0], Lat: pos[1], Text: s.String(), Style: scaleStyle(s)})
			}
		}
		if q.HasCenter() {
			rings = m.ripples(q)
			marks = append(marks, japanmap.Mark{Lon: q.Lon, Lat: q.Lat, Text: "X", Style: centerStyle})
		}
	}
	return m.worldMap.Render(coastStyle, rings, marks)
}

// ripples returns rings spreading out from the hypocenter.
// Bigger quakes spread further.
func (m model) ripples(q quake.Quake) []japanmap.Ring {
	elapsed := time.Since(m.rippleStart)
	if elapsed > rippleLength {
		return nil
	}
	reach := m.worldMap.DotsPerDegree() * max(1, q.Magnitude) * 0.6 // in dots
	color := scaleColors[q.MaxScale]
	if color == "" {
		color = "#FF3030"
	}
	style := lipgloss.NewStyle().Foreground(color)

	var rings []japanmap.Ring
	const waves = 3
	for i := 0; i < waves; i++ {
		// Each wave starts a little after the previous one.
		t := (elapsed.Seconds() - float64(i)*0.4) / (rippleLength.Seconds() - 0.8)
		if t <= 0 || t >= 1 {
			continue
		}
		rings = append(rings, japanmap.Ring{Lon: q.Lon, Lat: q.Lat, Radius: t * reach, Style: style})
	}
	return rings
}

func (m model) viewPanel() string {
	var b strings.Builder
	_, rows := m.mapSize()

	if m.err != nil {
		b.WriteString("error: " + m.err.Error() + "\n\n")
	}
	header := fmt.Sprintf("%s · %d件", m.filter.label(), len(m.visible))
	if m.loading {
		header += " (読み込み中)"
	}
	header = dimStyle.Render(header) + "\n\n"

	if len(m.visible) == 0 {
		msg := "条件に合う地震はありません"
		if m.loading || (len(m.all) == 0 && m.err == nil) {
			msg = "読み込み中..."
		}
		b.WriteString(header + dimStyle.Render(msg))
		return lipgloss.NewStyle().Width(panelWidth).Height(rows).Render(b.String())
	}

	q := m.visible[m.sel]
	b.WriteString(titleStyle.Render(placeName(q)) + "\n")
	b.WriteString(q.Time.Format("2006/01/02 15:04") + "\n\n")
	b.WriteString("最大震度  " + scaleStyle(q.MaxScale).Render(" "+q.MaxScale.Japanese()+" ") + "\n")
	b.WriteString("規模      " + magnitude(q) + "\n")
	b.WriteString("深さ      " + depth(q) + "\n")
	b.WriteString("津波      " + tsunami(q.Tsunami) + "\n\n")
	b.WriteString(header)

	// Show as many list rows as fit below the details.
	listRows := rows - 11
	start := max(0, min(m.sel-listRows/2, len(m.visible)-listRows))
	for i := start; i < len(m.visible) && i < start+listRows; i++ {
		qq := m.visible[i]
		line := fmt.Sprintf("%s %s %s",
			qq.Time.Format("01/02 15:04"),
			scaleStyle(qq.MaxScale).Render(fmt.Sprintf("%-2s", qq.MaxScale.String())),
			truncate(placeName(qq), panelWidth-17))
		if i == m.sel {
			line = selStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		b.WriteString(line + "\n")
	}
	return lipgloss.NewStyle().Width(panelWidth).Height(rows).Render(b.String())
}

func (m model) viewFooter() string {
	status := m.status
	if m.opts.Demo {
		status += " (replaying recent reports)"
	}
	return dimStyle.Render(fmt.Sprintf(" %s · ↑↓ select · enter replay · p period · s intensity · q quit · data: 気象庁防災情報XML", status))
}

func placeName(q quake.Quake) string {
	if q.Place == "" {
		return "震源調査中"
	}
	return q.Place
}

func magnitude(q quake.Quake) string {
	if q.Magnitude < 0 {
		return "不明"
	}
	return fmt.Sprintf("M%.1f", q.Magnitude)
}

func depth(q quake.Quake) string {
	switch {
	case !q.HasCenter() || q.Depth < 0:
		return "不明"
	case q.Depth == 0:
		return "ごく浅い"
	default:
		return fmt.Sprintf("%dkm", q.Depth)
	}
}

func tsunami(s string) string {
	switch s {
	case "None":
		return "なし"
	case "NonEffective":
		return "若干の海面変動"
	case "Watch":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FFE600")).Render("津波注意報")
	case "Warning":
		return centerStyle.Render("津波警報")
	case "Checking":
		return "調査中"
	default:
		return "不明"
	}
}

// truncate shortens s to fit in width terminal cells (Japanese characters are 2 cells wide).
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
