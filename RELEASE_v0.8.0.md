# Release v0.8.0

## New Features

### Disk Usage Monitoring in System Resources Panel

**What's New:**
- System Resources panel now displays root filesystem (/) disk space usage
- Shows used/total space with a visual percentage bar
- Uses the same compact, color-coded format as Memory and Swap metrics

**Display Format:**
```
Dsk [||||||                        15.9%] 66.12 GB/444.00 GB
```

**Color Coding:**
- 🟢 Green: < 60% used
- 🟡 Yellow: 60-79% used
- 🟠 Orange: 80-94% used
- 🔴 Red: ≥ 95% used

**Position:**
- Located between Swap and Disk I/O metrics for logical grouping of storage resources

**Benefits:**
- Quick visibility into available disk space without leaving ccdash
- Early warning when disk usage approaches capacity
- Consistent with existing memory and swap monitoring

**Technical Details:**
- Monitors root filesystem (`/`) using `gopsutil/v3/disk.Usage()`
- Real-time updates every 2 seconds (same as other system metrics)
- Help text updated (press `h` in ccdash to view)

## Installation

### Linux AMD64
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.8.0/ccdash-linux-amd64
chmod +x ccdash-linux-amd64
sudo mv ccdash-linux-amd64 /usr/local/bin/ccdash
```

### Linux ARM64
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.8.0/ccdash-linux-arm64
chmod +x ccdash-linux-arm64
sudo mv ccdash-linux-arm64 /usr/local/bin/ccdash
```

### macOS AMD64
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.8.0/ccdash-darwin-amd64
chmod +x ccdash-darwin-amd64
sudo mv ccdash-darwin-amd64 /usr/local/bin/ccdash
```

### macOS ARM64 (Apple Silicon)
```bash
wget https://github.com/jedarden/ccdash/releases/download/v0.8.0/ccdash-darwin-arm64
chmod +x ccdash-darwin-arm64
sudo mv ccdash-darwin-arm64 /usr/local/bin/ccdash
```

### Self-Update (Existing Users)

If you already have ccdash installed:

```bash
# ccdash will notify you when an update is available
# Press 'u' in the dashboard to auto-update to v0.8.0
```

Or manually update:

```bash
# Check current version
ccdash --version

# Update to latest
ccdash --update
```

## Checksums

```
9be4b99656720c4d1474b738231d5c9002b0f2a33c5035a7d701d7129140e919  ccdash-linux-amd64
dd3288a0f943578c24f25e3a64a2c1cbcf420c9553cbb33d92dd2f5dab252444  ccdash-linux-arm64
c909f307660c0d7e3341e97b13fd19a289964a5382be395c99a5c5b78044b8f7  ccdash-darwin-amd64
9b607644f228b889aa7c3d2a1ad6600c54eeb1eea9900855c58bd7e18e1bc4af  ccdash-darwin-arm64
```

## Full Changelog

See [CHANGELOG.md](CHANGELOG.md) for complete version history.

## Upgrading from v0.7.23

No breaking changes. Simply replace your binary with the new version.

The disk usage metric will appear automatically in the System Resources panel.
