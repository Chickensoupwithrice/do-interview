# DigitalOcean Droplet deployment

SQLite needs durable local disk, so the simplest safe DigitalOcean deployment for this build is a single Droplet instead of App Platform.

## 1. Create a Droplet

- Ubuntu 24.04
- Basic shared CPU is enough for the exercise
- Open inbound `22`, `80`, and `443`

## 2. Install Docker on the Droplet

```bash
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
```

Reconnect after adding yourself to the `docker` group.

## 3. Clone and run the service

```bash
git clone https://github.com/YOUR_GITHUB_USERNAME/url-shortener.git
cd url-shortener
docker build -t url-shortener .
mkdir -p data
docker run -d \
  --name url-shortener \
  -p 127.0.0.1:8080:8080 \
  -e BASE_URL=https://YOUR_DOMAIN \
  -e DATABASE_PATH=/app/data/shortener.db \
  -v "$PWD/data:/app/data" \
  --restart unless-stopped \
  url-shortener
```

## 4. Put TLS in front with Caddy

Point your domain at the Droplet, then run Caddy for HTTPS termination:

```bash
docker run -d \
  --name caddy \
  -p 80:80 \
  -p 443:443 \
  -v caddy_data:/data \
  -v caddy_config:/config \
  -v "$PWD/Caddyfile:/etc/caddy/Caddyfile:ro" \
  --restart unless-stopped \
  caddy:2
```

Example `Caddyfile`:

```text
YOUR_DOMAIN {
    reverse_proxy 127.0.0.1:8080
}
```
