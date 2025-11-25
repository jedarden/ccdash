# ccdash v0.1.4 Release Notes

**Release Date:** 2025-11-21

## 🎉 What's New in v0.1.4

This release focuses on fixing panel rendering issues and improving the overall user experience with better tmux session detection and helpful documentation.

### 🐛 Critical Bug Fixes

- **Fixed panel width calculations** - Panels now properly account for lipgloss padding (0,1), ensuring correct rendering without right-side cutoff in all terminal widths
- **Improved terminal compatibility** - Works correctly in 202-character wide terminals and other common sizes

### ✨ New Features

- **Version display in status bar** - Shows version number in bottom left as "HH:MM:SS vX.X.X"
- **Enhanced tmux session detection** - Using proven patterns from unified-dashboard:
  - 🟢 **WORKING**: Detects "Finagling...", "Puzzling...", "Listing...", "Analyzing", "Processing", and more
  - 🔴 **READY**: Identifies prompt patterns like "⏵⏵ bypass permissions" and "Claude Code ❯"
  - 🟡 **ACTIVE**: Shows when user is actively in the session
  - ⚠️ **ERROR**: Checks last 5 lines for error patterns
- **Help mode** - Press `h` to cycle through detailed explanations for each panel
- **Idle duration tracking** - Shows how long each tmux session has been inactive
- **Dynamic CPU display** - ≤6 cores: one per line with full-width bars; >6 cores: multiple per line
- **Smart help layout** - Automatically uses 2-column layout when help text is long

### 📝 Documentation Improvements

- Added comprehensive **CHANGELOG.md** with full version history
- Updated **README.md** with:
  - Pre-built binary installation instructions
  - SHA256 checksum verification guide
  - Improved keyboard controls section
  - Display features documentation
- Detailed release notes for easy reference

### 🔧 Technical Improvements

- Fixed `.gitignore` to properly exclude only the binary, not source code
- Set default branch to `main`
- Optimized tmux pane capture to last 15 lines (down from 50) for better performance
- Content change detection with timing rules for accurate status
- Proper panel width calculation: `totalPanelWidth = d.width - 6` to account for padding

## 📦 Installation

### Quick Install (Linux)

```bash
# Download the binary
curl -LO https://github.com/jedarden/ccdash/releases/download/v0.1.4/ccdash-linux-amd64

# Download the checksum
curl -LO https://github.com/jedarden/ccdash/releases/download/v0.1.4/ccdash-linux-amd64.sha256

# Verify integrity
sha256sum -c ccdash-linux-amd64.sha256

# Make executable
chmod +x ccdash-linux-amd64

# Optional: Move to PATH
sudo mv ccdash-linux-amd64 /usr/local/bin/ccdash

# Run it
ccdash
```

### Binary Verification

**SHA256 Checksum:**
```
626487d87a2117c5f96454ad2a2f8ed1ed4fee97ce8051810c2fb1b6a7db844e  ccdash-linux-amd64
```

To verify:
```bash
sha256sum -c ccdash-linux-amd64.sha256
```

Expected output: `ccdash-linux-amd64: OK`

### Alternative Installation Methods

**Using Go:**
```bash
go install github.com/jedarden/ccdash/cmd/ccdash@v0.1.4
```

**From Source:**
```bash
git clone https://github.com/jedarden/ccdash.git
cd ccdash
git checkout v0.1.4
make build
./bin/ccdash
```

## 🎮 Usage

Simply run the binary:
```bash
ccdash
```

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `q` or `Ctrl+C` | Quit the application |
| `r` | Refresh metrics immediately |
| `h` | Cycle through help mode (explains each panel) |

### Display Features

- **Responsive Layout**: Automatically adjusts to your terminal size
  - Ultra-wide (≥240 cols): 3 panels side-by-side
  - Wide (120-239 cols): 2 panels top, 1 bottom
  - Narrow (<120 cols): Panels stacked vertically

- **Tmux Status Indicators**:
  - 🟢 **WORKING** - Claude Code actively processing
  - 🔴 **READY** - Waiting for user input at prompt
  - 🟡 **ACTIVE** - User actively in session
  - ⚠️ **ERROR** - Error state detected

## 📋 Requirements

- **Terminal**: Minimum 80x24, true color recommended
- **tmux** (optional): For session monitoring
- **Claude Code** (optional): For token usage tracking from `~/.claude/projects`

## 🔄 Upgrading from v0.1.0

Simply download the new binary and replace your existing one:

```bash
# Backup current version (optional)
mv /usr/local/bin/ccdash /usr/local/bin/ccdash.old

# Download and install new version
curl -LO https://github.com/jedarden/ccdash/releases/download/v0.1.4/ccdash-linux-amd64
chmod +x ccdash-linux-amd64
sudo mv ccdash-linux-amd64 /usr/local/bin/ccdash

# Verify version
ccdash --version
```

## 🐛 Known Issues

None reported for this release.

## 💬 Feedback & Support

- **Issues**: [GitHub Issues](https://github.com/jedarden/ccdash/issues)
- **Source**: [GitHub Repository](https://github.com/jedarden/ccdash)

## 🙏 Acknowledgments

This release was developed with assistance from Claude Code, demonstrating the power of AI-assisted development.

Built with:
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [gopsutil](https://github.com/shirou/gopsutil) - System metrics

---

**Full Changelog**: [CHANGELOG.md](../../CHANGELOG.md)

🚀 **Generated with [Claude Code](https://claude.com/claude-code)**

Co-Authored-By: Claude <noreply@anthropic.com>
