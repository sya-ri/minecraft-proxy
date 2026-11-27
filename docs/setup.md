# Setup

## Cloud accounts

1. Enable the Compute Engine API and billing for your Google Cloud project. The signed-in user needs permission to create and delete instances and their boot disks, attach an external IPv4 address, and update VM scheduling. On a standard project with its own VPC, start with the [Compute Instance Admin role](https://docs.cloud.google.com/compute/docs/access/iam#compute.instanceAdmin.v1).
2. In the VPC used by the app, allow inbound TCP 7000 and 25565 on instances tagged `minecraft-proxy`. Add each public UDP port you plan to use. Restrict TCP 7000 to your app's outbound address when it is stable. [VPC firewall rules](https://docs.cloud.google.com/firewall/docs/using-firewalls)
3. Create a Google OAuth client of type **Desktop app**. If the consent screen is in testing, add the operator as a test user. [Google desktop OAuth](https://developers.google.com/identity/protocols/oauth2/native-app)
4. Create a Cloudflare OAuth client using authorization code, token authentication **None**, and the redirect `http://127.0.0.1:42813/callback`. Allow Zone Read and DNS Write. A private client is enough for members of its Cloudflare account. [Cloudflare OAuth clients](https://developers.cloudflare.com/fundamentals/oauth/create-an-oauth-client/)

## Relays

1. Start your local Minecraft Java server. For a separate UDP service, start it on its local UDP port.
2. In **Settings**, enter the project, VPC, Cloudflare zone, public hostname, local ports, and OAuth client IDs. Sign in to Google and Cloudflare. Access and refresh tokens are stored in your OS credential store.
3. Select **Start**. When the relay is online, enter the public hostname in Minecraft. External Java connections use port 25565. External UDP connections use the configured UDP port.
4. Select **Extend 4 h** to add four hours to the running relay's expiry. Select **Stop** to remove its DNS record and delete its VM and boot disk.

Add one relay profile per hostname with **Add relay profile**. Set a different local TCP port for each local server. Every active profile starts its own VM and accrues its own relay hours. Keep the desktop app and local servers running while people are connected.

**Replace existing DNS records** removes conflicting A, AAAA, CNAME, and Java SRV records when the new relay is ready. Stopping removes the app's A record. The original records must be restored separately if needed. Cached DNS can delay a hostname update or removal.

## Build from source

Use the Go, Node, and pnpm versions in `mise.toml`. Linux needs GTK4, WebKitGTK 6.0, and a running Secret Service provider for OAuth credentials. Windows needs the WebView2 Runtime. Build on the target OS and CPU architecture to bundle the matching frp client.

```sh
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend build
node scripts/bundle-frp.mjs
go test ./...
go build -tags production -o build/minecraft-proxy .
```

Use `build/Minecraft-Proxy.exe` as the output on Windows. The [desktop workflow](../.github/workflows/desktop.yml) builds packaged macOS and Linux artifacts and uses the Windows GUI linker option.

```sh
pnpm --dir frontend exec playwright install chromium
pnpm --dir frontend test
```
