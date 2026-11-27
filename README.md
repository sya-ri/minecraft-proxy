# Minecraft Proxy

Minecraft Proxy is a planned desktop app that exposes a local Minecraft Java server through a temporary Google Compute Engine (GCE) relay. Players connect using a domain you manage in Cloudflare. The app and downloads are in development.

The [v1 branch](https://github.com/sya-ri/minecraft-proxy/tree/v1) contains the Docker Compose setup for routing to multiple Minecraft servers through an existing public server.

## Service comparison

✓ Supported · △ With limitations · ✗ Unavailable · ? Unverified. List prices in USD, checked November 27, 2026.

| Service | Price | Stable public endpoint | Your own hostname | Java list ping on your hostname | UDP |
| --- | --- | --- | --- | --- | --- |
| [ngrok Free](https://ngrok.com/pricing) | Free with card verification for TCP | ✗ Random hostname and port | △ A TCP DNS alias needs updating when the endpoint changes | ? | ✗ [TCP only](https://ngrok.com/compare/tailscale) |
| [ngrok Hobbyist](https://ngrok.com/docs/gateway/domains/tcp-addresses) | US$10/month on monthly billing | ✓ One reserved TCP address | △ [CNAME with assigned port](https://ngrok.com/docs/gateway/domains/custom-domains#using-custom-domains-with-tcp-endpoints), or Java SRV | ? | ✗ TCP only |
| [Playit Free](https://playit.gg/) | Free | ✓ Assigned hostname and port for the lifetime of the tunnel | △ Java SRV workaround | △ Worked on the assigned name. Failed with the custom SRV setup tested | ✓ Standalone UDP tunnels |
| [Playit Premium](https://playit.gg/support/playit-premium/) | From US$3/month | ✓ Assigned hostname and port | ✓ [External domain registration](https://playit.gg/support/external-domain-cloudflare/) | ? | ✓ Generic TCP+UDP tunnels |
| Minecraft Proxy (planned) | US$0.0165/h in Tokyo · US$0.0139/h in Iowa | ✓ Stable hostname. IP can change between sessions | ✓ Cloudflare DNS, direct TCP 25565 | ✓ Planned through the custom hostname | ✓ TCP and optional UDP on one relay |

ngrok's full custom-domain registration starts at US$20/month on Pay-as-you-go. TCP aliases use an assigned TCP address. [ngrok pricing](https://ngrok.com/pricing)

The Playit Free observations are from one tunnel with a Minecraft Java 1.21.11 server. The assigned name returned a server-list ping. The custom SRV name reached the start of login, but its list ping was reset.

### Monthly relay cost

![Relay costs reach Playit Premium's US$3/month starting price at about 182 hours in Tokyo and 215 hours in Iowa.](docs/monthly-relay-cost.svg)

At Playit Premium's US$3/month starting price, break-even is **182 hours in Tokyo** (`asia-northeast1`) and **215 hours in Iowa** (`us-central1`). Tokyo is the app's relay region. Iowa is the default region in Google Cloud's pricing table. [Playit pricing](https://playit.gg/)

Both estimates use an `e2-micro` (two shared vCPUs and 1 GiB RAM), a 10 GiB standard boot disk, and an ephemeral public IPv4 address. Add up the uptime of all relays. The estimates use on-demand list rates, assume the VM and disk are deleted between sessions, and exclude free-tier credits, network transfer, taxes, and domain registration. [GCE compute pricing](https://cloud.google.com/products/compute/pricing/general-purpose) · [Disk pricing](https://cloud.google.com/compute/disks-image-pricing) · [IPv4 and transfer pricing](https://cloud.google.com/vpc/network-pricing#ipaddress)

## Design

[Requirements and design](docs/design.md)
