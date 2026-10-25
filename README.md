# yure

ターミナルで、日本で起きた地震を地図で見る TUI です。気象庁の情報を1分ごとに取得して表示します。

*A terminal UI that shows recent earthquakes in Japan on a map, updated every minute.*

![yure --demo の画面: 地震が届くたびに震源から波紋が広がる](docs/demo.gif)

- 点字文字(Braille)で描いた日本地図に、震源と都道府県ごとの震度を表示
- 新しい地震が届くと、震源から波紋が広がる
- 期間(24時間 / 7日)と震度で絞り込み
- データは**気象庁の公式配信(気象庁防災情報XML)のみ**を使用

> [!WARNING]
> **防災目的には使わないでください。** yure は個人の趣味のプロジェクトです。
> 情報の遅延・欠落・誤表示が起こりえます。避難などの判断には、気象庁や自治体などの公式な情報を使ってください。
> 緊急地震速報には対応していません。新しい地震の表示には、発表から最大1〜2分かかります。

## インストール

macOS / Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/torihazi/yure/main/install.sh | sh
```

最新のリリースをダウンロードし、チェックサムを確かめてから `~/.local/bin/yure` に置きます。置き場所は `YURE_INSTALL_DIR` で変えられます。

スクリプトを使いたくない場合は、[Releases](https://github.com/torihazi/yure/releases) からダウンロードして展開してください(macOS で「開けません」と出たら `xattr -d com.apple.quarantine ./yure`)。

macOS で動作を確認しています。Linux はビルドとインストールのみ確認しており、動作は保証できません(不具合があれば Issue で教えてください)。

## 使い方

```sh
yure                 # 最新の地震を表示する
yure --demo          # 直近の実際の地震を数秒おきに再生する
yure --version       # バージョンを表示する
```

地震は1日に数回しか起きないので、まずは `--demo` で動きを見るのがおすすめです。

### キー操作

| キー | 動作 |
| --- | --- |
| `↑` `↓` / `k` `j` | 地震を選ぶ |
| `Enter` | 波紋をもう一度再生する |
| `p` | 期間を切り替える(24時間 ⇄ 7日) |
| `s` | 震度の下限を切り替える(全震度 → 2以上 → 3以上 → 4以上 → 5弱以上) |
| `q` | 終了する |

## 注意

- **更新は1分ごとです。** 気象庁のフィードを60秒ごとに確認しています(フィード自体が60秒間キャッシュされるため、それより速くしても意味がありません)。
- **表示できるのは直近約7日分です。** 気象庁の長期フィードに載っている範囲です。
- **震度は都道府県単位です。** 各都道府県で最も強い震度を、県庁所在地の位置に表示しています。
- 国内で揺れを観測していない地震(遠地地震など)は表示しません。
- 取得した電文は `~/Library/Caches/yure/jmaxml`(macOS の場合)にキャッシュし、8日経ったものは起動時に削除します。
- 地図の表示範囲には、先島諸島と小笠原諸島が含まれていません。

## データの出典

- 地震情報: [気象庁防災情報XML](https://xml.kishou.go.jp/)(出典:気象庁)。yure が電文を独自に解析・加工して表示しています。
- 海岸線: [Natural Earth](https://www.naturalearthdata.com/)(パブリックドメイン)

地図データは `tools/genmap` で生成しています。README の GIF は [VHS](https://github.com/charmbracelet/vhs) で `docs/demo.tape` から録画しています。

```sh
curl -LO https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_50m_land.geojson
go run ./tools/genmap ne_50m_land.geojson > internal/japanmap/coast_gen.go
```

## ライセンス

[MIT](LICENSE)
