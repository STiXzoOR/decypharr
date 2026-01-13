# Zurg Setup Guide

This guide covers how to set up Zurg with Decypharr for an optimized Real-Debrid workflow.

## Overview

### What is Zurg?

[Zurg](https://github.com/debridmediamanager/zurg-testing) is a tool that provides a WebDAV interface to Real-Debrid, allowing you to mount your Real-Debrid library as a filesystem using rclone.

### How Decypharr Works with Zurg

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  Sonarr/    │────▶│  Decypharr  │────▶│ Real-Debrid │
│  Radarr     │     │  (QBit API) │     │    API      │
└─────────────┘     └──────┬──────┘     └─────────────┘
                           │
                           │ Creates symlinks
                           ▼
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Plex/     │────▶│   Symlink   │────▶│    Zurg     │
│   Jellyfin  │     │   Folder    │     │   Mount     │
└─────────────┘     └─────────────┘     └─────────────┘
```

1. **Decypharr** receives download requests from *Arr apps via QBittorrent API
2. **Decypharr** adds torrents to Real-Debrid and waits for completion
3. **Decypharr** creates symlinks pointing to the Zurg mount
4. **Zurg** provides file access via WebDAV, mounted by rclone
5. **Media servers** read files through the symlinks

### Why Use Zurg?

- **Faster file discovery**: Zurg caches Real-Debrid's file structure
- **Reduced API calls**: Single WebDAV mount instead of per-file API requests
- **Better compatibility**: Works seamlessly with Plex, Jellyfin, and other media servers
- **Repair optimization**: Decypharr can use Zurg's HTTP API for faster broken file detection

## Prerequisites

- Docker and Docker Compose
- Real-Debrid account with API token
- Basic understanding of rclone mounts

## Directory Structure

Create the following directory structure:

```bash
mkdir -p /opt/zurg/config
mkdir -p /mnt/zurg          # Zurg rclone mount
mkdir -p /mnt/symlinks      # Decypharr symlinks
```

## Zurg Setup

### Configuration

Create `/opt/zurg/config/config.yml`:

```yaml
token: YOUR_REALDEBRID_API_TOKEN

host: "0.0.0.0"
port: 9999

# Directory structure in the mount
directories:
  torrents:
    group: 1
    filters:
      - regex: /.*/
```

!!! tip "Advanced Directory Configuration"
    Zurg supports organizing files into different directories based on regex patterns. See the [Zurg documentation](https://github.com/debridmediamanager/zurg-testing) for advanced configuration.

### Docker Compose

Create `docker-compose.yml`:

```yaml
version: "3.8"

services:
  zurg:
    image: ghcr.io/debridmediamanager/zurg-testing:latest
    container_name: zurg
    restart: unless-stopped
    ports:
      - "9999:9999"
    volumes:
      - /opt/zurg/config:/config
    environment:
      - TZ=UTC

  rclone:
    image: rclone/rclone:latest
    container_name: zurg-rclone
    restart: unless-stopped
    cap_add:
      - SYS_ADMIN
    security_opt:
      - apparmor:unconfined
    devices:
      - /dev/fuse
    volumes:
      - /mnt/zurg:/data:rshared
      - /opt/zurg/rclone:/config
    command: >
      mount zurg: /data
      --config=/config/rclone.conf
      --allow-other
      --allow-non-empty
      --dir-cache-time=10s
      --vfs-cache-mode=full
      --vfs-cache-max-age=24h
      --vfs-read-chunk-size=32M
      --vfs-read-chunk-size-limit=256M
      --buffer-size=32M
    depends_on:
      - zurg

  decypharr:
    image: ghcr.io/sirrobot01/decypharr:latest
    container_name: decypharr
    restart: unless-stopped
    ports:
      - "8282:8282"
    volumes:
      - /opt/decypharr:/config
      - /mnt/zurg:/mnt/zurg:rshared      # Zurg mount (read)
      - /mnt/symlinks:/mnt/symlinks       # Symlink folder (write)
    environment:
      - TZ=UTC
    depends_on:
      - rclone
```

### Rclone Configuration

Create `/opt/zurg/rclone/rclone.conf`:

```ini
[zurg]
type = webdav
url = http://zurg:9999/dav
vendor = other
pacer_min_sleep = 0
```

## Decypharr Configuration

Configure Decypharr to use the Zurg mount in `/opt/decypharr/config.json`:

```json
{
  "debrids": [
    {
      "name": "realdebrid",
      "api_key": "YOUR_REALDEBRID_API_TOKEN",
      "folder": "/mnt/zurg/torrents"
    }
  ],
  "qbittorrent": {
    "download_folder": "/mnt/symlinks",
    "categories": ["radarr", "sonarr", "tv", "movies"]
  },
  "repair": {
    "enabled": true,
    "zurg_url": "http://zurg:9999",
    "interval": "1h"
  }
}
```

### Key Configuration Options

| Option | Description |
|--------|-------------|
| `debrids[].folder` | Path where Zurg is mounted. Decypharr looks for torrents here. |
| `qbittorrent.download_folder` | Where Decypharr creates symlinks pointing to the Zurg mount. |
| `repair.zurg_url` | Zurg's HTTP URL for optimized repair checks (optional). |

## Start the Stack

```bash
docker-compose up -d
```

Verify everything is running:

```bash
# Check Zurg is responding
curl http://localhost:9999/http/version.txt

# Check mount is working
ls /mnt/zurg/torrents

# Check Decypharr
curl http://localhost:8282/version
```

## Configure *Arr Applications

In Sonarr/Radarr, add Decypharr as a download client:

1. Go to **Settings → Download Clients**
2. Add **qBittorrent**
3. Configure:
   - **Host**: `decypharr` (or IP/hostname)
   - **Port**: `8282`
   - **Username/Password**: As configured in Decypharr (if auth enabled)

## How Symlinks Work

When a download completes:

1. Decypharr adds torrent to Real-Debrid
2. Real-Debrid caches/downloads the files
3. Zurg's cache refreshes (files appear in `/mnt/zurg/torrents/`)
4. Decypharr creates symlink:
   ```
   /mnt/symlinks/radarr/Movie.Name.2024.mkv → /mnt/zurg/torrents/Movie.Name.2024/Movie.Name.2024.mkv
   ```
5. *Arr app imports from the symlink path

## Repair Worker with Zurg

The repair worker can use Zurg's HTTP API to check file availability without streaming data:

```json
{
  "repair": {
    "enabled": true,
    "zurg_url": "http://zurg:9999",
    "interval": "1h"
  }
}
```

### How It Works

Without `zurg_url`, Decypharr must make HTTP requests to Real-Debrid for each file to check availability.

With `zurg_url`, Decypharr queries:
```
http://zurg:9999/http/__all__/{torrent_name}/{file_path}
```

This is faster because:
- Zurg caches the file structure locally
- No data is actually streamed, just availability checked
- Reduces Real-Debrid API usage

## Troubleshooting

### Mount Not Working

```bash
# Check if FUSE is available
ls -la /dev/fuse

# Check rclone logs
docker logs zurg-rclone

# Verify Zurg is accessible from rclone container
docker exec zurg-rclone curl http://zurg:9999/http/version.txt
```

### Symlinks Not Created

1. Verify the Zurg mount path matches `debrids[].folder`:
   ```bash
   ls /mnt/zurg/torrents
   ```

2. Check Decypharr logs:
   ```bash
   docker logs decypharr
   ```

3. Ensure the torrent completed in Real-Debrid

### Files Not Appearing in Zurg

```bash
# Force Zurg to refresh
curl -X POST http://localhost:9999/http/refresh

# Check Zurg logs
docker logs zurg
```

### Permission Issues

Ensure proper permissions:

```bash
# The user running Docker needs access to mount points
sudo chown -R $USER:$USER /mnt/zurg /mnt/symlinks

# Or use specific UID/GID in Docker
```

### Slow File Access

Tune rclone cache settings:

```yaml
command: >
  mount zurg: /data
  --vfs-cache-mode=full
  --vfs-cache-max-age=48h        # Increase cache age
  --vfs-cache-max-size=50G       # Set cache size limit
  --vfs-read-chunk-size=64M      # Larger chunks
  --buffer-size=64M
```

## Alternative: Decypharr Internal WebDAV

Decypharr has a built-in WebDAV server that can be used instead of Zurg. This is simpler but may have fewer features:

```json
{
  "debrids": [
    {
      "name": "realdebrid",
      "api_key": "YOUR_API_KEY",
      "folder": "/mnt/remote/realdebrid",
      "use_webdav": true
    }
  ]
}
```

See the [Internal Mounting Guide](internal-mounting.md) for details.

## Next Steps

- [Configure Reverse Proxy](reverse-proxy.md) for external access
- [Set up Repair Worker](../features/repair-worker.md) for automated maintenance
- [Manual Downloading](downloading.md) for adding torrents directly
