# Minecraft Proxy

Minecraft Proxy is a planned desktop app that exposes a local Minecraft Java server through a temporary Google Compute Engine (GCE) relay. Players connect using a domain you manage in Cloudflare. The app and downloads are in development.

The [v1 branch](https://github.com/sya-ri/minecraft-proxy/tree/v1) contains the Docker Compose setup for routing to multiple Minecraft servers through an existing public server.

## Service comparison

List prices in USD, checked November 27, 2026.

| Service | Price | Stable address and custom domain | Minecraft Java server-list ping | UDP |
| --- | --- | --- | --- | --- |
| [ngrok Free](https://ngrok.com/pricing) | Free. TCP endpoints require card verification | Random TCP hostname and port. A DNS alias needs updating when the assignment changes | Raw TCP forwarding. A custom-domain ping was not tested | [Unavailable](https://ngrok.com/compare/tailscale) |
| [ngrok Hobbyist](https://ngrok.com/docs/gateway/domains/tcp-addresses) | US$10/month on monthly billing. Includes one reserved TCP address | Fixed assigned hostname and port. [Use a CNAME and the assigned port](https://ngrok.com/docs/gateway/domains/custom-domains#using-custom-domains-with-tcp-endpoints), or a Minecraft Java SRV record to omit the port | Raw TCP forwarding. A custom-domain ping was not tested | Unavailable |
| [Playit Free](https://playit.gg/) | Free | Keeps the assigned hostname and port for the lifetime of the tunnel. A Minecraft Java SRV record can send some clients to it using a custom name | The assigned name responded during testing. The tested custom-domain SRV setup reset the list ping | Standalone UDP tunnels are free |
| [Playit Premium](https://playit.gg/support/playit-premium/) | From US$3/month | [Register and assign a domain you own](https://playit.gg/support/external-domain-cloudflare/) | A formally registered custom domain was not tested | Includes generic combined TCP+UDP tunnels |
| Minecraft Proxy (design) | About US$0.07 for four hours in GCE Tokyo | Keeps the custom hostname and updates its Cloudflare A record at startup. Java uses TCP 25565 directly | Planned to support both ping and login through the custom hostname | TCP and UDP on the same relay |

ngrok's general custom-domain registration is a Pay-as-you-go feature (from US$20/month). Its [TCP alias](https://ngrok.com/docs/gateway/domains/custom-domains#using-custom-domains-with-tcp-endpoints) instead points your DNS at an assigned TCP address. [ngrok pricing](https://ngrok.com/pricing)

The Playit observations come from one free tunnel tested with a temporary Minecraft Java 1.21.11 server. The custom-domain SRV path reached the start of login and reset the server-list ping. The assigned Playit address passed both checks.

The estimate uses an `e2-micro` in Tokyo, a 10 GiB standard boot disk, and an ephemeral public IPv4 address. Transfer, taxes, and domain registration are separate. Deleting the VM and its boot disk and using an ephemeral IP avoids idle relay charges. [GCE compute and disk pricing](https://cloud.google.com/products/compute/pricing/general-purpose) · [Public IPv4 pricing](https://cloud.google.com/vpc/network-pricing#ipaddress)

## Design

[Requirements and design](docs/design.md)
