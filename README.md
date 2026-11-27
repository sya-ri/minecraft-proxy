# Minecraft Proxy v2

手元の Minecraft Java サーバーを、遊ぶときだけ作る Google Compute Engine（GCE）の中継 VM 経由で公開するデスクトップアプリの設計です。接続先には Cloudflare で管理している独自ドメインを使います。v2 の実装と配布は準備中です。

常設サーバーで複数の Minecraft サーバーを振り分ける Docker Compose 版は [v1 ブランチ](https://github.com/sya-ri/minecraft-proxy/tree/v1) にあります。

## サービス比較

料金は 2026 年 11 月 27 日に確認した米ドルの公開価格です。

| サービス | 料金 | 固定の接続先と独自ドメイン | Minecraft Java のサーバー一覧 | UDP |
| --- | --- | --- | --- | --- |
| [ngrok Free](https://ngrok.com/pricing) | 無料。TCP の利用にカード確認が必要 | TCP のホスト名とポートはランダム。独自ドメインから参照する場合は割り当てが変わるたびに DNS 更新が必要 | TCP 転送。独自ドメインでの一覧ピングは未検証 | [非対応](https://ngrok.com/compare/tailscale) |
| [ngrok Hobbyist](https://ngrok.com/docs/gateway/domains/tcp-addresses) | 月払い US$10。固定 TCP アドレス1個を含む | 固定のホスト名とポートに [CNAME と公開ポート](https://ngrok.com/docs/gateway/domains/custom-domains#using-custom-domains-with-tcp-endpoints) を使える。Java では SRV でポート入力を省略できる | TCP 転送。独自ドメインでの一覧ピングは未検証 | 非対応 |
| [Playit 無料](https://playit.gg/) | 無料 | 同じトンネルを使う間、割り当てられたホスト名とポートを使える。Java では独自ドメインから SRV で接続できる構成もある | 割り当て名で応答。検証した SRV 構成では、独自ドメインでの一覧ピングが切断された | UDP 単独のトンネルは無料 |
| [Playit Premium](https://playit.gg/support/playit-premium/) | 月額 US$3〜 | [所有する独自ドメインを正式に登録してトンネルへ割り当てられる](https://playit.gg/support/external-domain-cloudflare/) | 正式に登録した独自ドメインでの一覧ピングは未検証 | 汎用 TCP+UDP を1本にまとめられる |
| Minecraft Proxy v2（設計） | 東京の GCE で4時間あたり約 US$0.07 | 独自ドメインを維持し、起動時に Cloudflare の A レコードを更新。Java は TCP 25565 を使うため SRV は不要 | 独自ドメインで一覧ピングと接続を通す設計 | TCP と UDP を同じ VM で公開 |

ngrok の一般的な独自ドメイン登録は Pay-as-you-go（月額 US$20〜）です。表にある TCP 用の CNAME は、ngrok が割り当てた TCP アドレスを自分の DNS から参照する方法です。 [ngrok の料金](https://ngrok.com/pricing) · [TCP での独自ドメインの扱い](https://ngrok.com/docs/gateway/domains/custom-domains#using-custom-domains-with-tcp-endpoints)

Playit 無料版の結果は、一時サーバーを使った Minecraft Java 1.21.11 の検証に基づきます。独自ドメインではログイン開始まで応答し、同じ名前での一覧ピングは切断されました。検証範囲は、このトンネル構成です。

v2 の試算は東京の `e2-micro`、標準ブートディスク 10 GiB、一時 IPv4 を4時間使った場合です。税・転送量・ドメイン代は別です。VM とディスクを削除し、固定 IP を確保しない構成では、利用しない月の中継リソース費用は発生しません。 [GCE の料金](https://cloud.google.com/products/compute/pricing/general-purpose) · [公開 IPv4 の料金](https://cloud.google.com/vpc/network-pricing#ipaddress)

## 設計

[要件・設計](docs/design.md)
