# Changelog

All notable changes to ccdash will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0] - 2025-11-27

### Added
- **Self-update functionality**: ccdash now checks for updates automatically from GitHub releases
  - Status bar shows "⬆ vX.X.X available! Press u to update" when a new version exists
  - Press `u` to download and apply the update in-place
  - Automatic version comparison with GitHub releases API
- **Per-model cost tracking**: Token panel now shows individual costs for each Claude model
  - Displays model name with cost and token count
  - Color-coded by model type (Opus=red, Sonnet=cyan, Haiku=green)
  - Sorted by cost (highest first)
  - Smart model name shortening (e.g., "claude-opus-4-5-20251101" → "Opus 4.5")
- **Improved CPU core display alignment**
  - Square brackets now align consistently across all core displays
  - Fixed-width labels ensure proper column alignment
  - Consistent bar width calculation matching memory/swap lines

### Changed
- CPU total bar now uses the same width calculation as Memory and Swap for visual consistency
- Status bar dynamically shows available shortcuts based on update availability
- Token panel now includes empty line separator before per-model breakdown

### Technical Details
- New `internal/updater` package for update management
- Added `ModelUsage` struct for per-model token and cost tracking
- Updater uses GitHub API with 5-minute cache interval
- Self-update uses atomic file replacement with restart script

## [0.1.4] - 2025-11-21

### Fixed
- Fixed panel width calculation to properly account for padding, ensuring panels fit exactly in terminal width
- Panels now render correctly in 202-character wide terminals without right-side cutoff

### Changed
- Adjusted panel width distribution to account for lipgloss padding (0,1)
- Updated width calculation: totalPanelWidth = d.width - 6 (to account for 2 chars padding per panel)

## [0.1.3] - 2025-11-21

### Fixed
- Narrowed tmux sessions panel by additional character to prevent overflow
- Improved panel border calculations

## [0.1.2] - 2025-11-21

### Fixed
- Narrowed tmux sessions panel by 3 characters to better fit terminal width
- Fixed right-side cutoff issues in ultra-wide mode

## [0.1.1] - 2025-11-21

### Added
- Version display in status bar (bottom left)
- Version now shows as "HH:MM:SS vX.X.X" format

### Changed
- Updated help pane width calculation to match normal view (d.width - 2)

## [0.1.0] - 2025-11-21

### Added
- Initial release of ccdash
- Real-time system resource monitoring (CPU, memory, swap, disk I/O, load averages)
- Claude Code token usage tracking from ~/.claude/projects
- Tmux session monitoring with intelligent status detection
- Beautiful TUI with responsive layout modes:
  - Ultra-wide mode (≥240 cols): 3 panels side-by-side
  - Wide mode (120-239 cols, ≥30 lines): 2 panels top, 1 bottom
  - Narrow mode (<120 cols): panels stacked vertically
- Help mode (press 'h') with cycling explanations for each panel
- Smart tmux session status detection:
  - 🟢 WORKING - Claude Code actively processing
  - 🔴 READY - Waiting for user input at prompt
  - 🟡 ACTIVE - User actively in session
  - ⚠️ ERROR - Error state or undefined condition
- Detection patterns from unified-dashboard:
  - Working indicators: "Finagling...", "Puzzling...", "Listing...", etc.
  - Prompt patterns: "⏵⏵ bypass permissions", "Claude Code" + "❯"
  - Error detection in last 5 lines only
- Idle duration tracking for tmux sessions
- Dynamic CPU core display (≤6 cores: one per line, >6: multiple per line)
- 2-column help layout when text exceeds available lines
- Keyboard shortcuts:
  - q, Ctrl+C: Quit
  - r: Refresh metrics immediately
  - h: Cycle through help mode
- Status bar with time, github link, dimensions, and shortcuts
- Color-coded metrics with 4-tier thresholds
- Unified-dashboard inspired styling with vertical bars and emojis

### Technical Details
- Built with Bubble Tea TUI framework
- Uses lipgloss for terminal styling
- gopsutil for system metrics collection
- Captures last 15 lines of tmux panes for status detection
- Content change detection with timing rules
- 2-second refresh interval for metrics

[0.3.0]: https://github.com/jedarden/ccdash/releases/tag/v0.3.0
[0.1.4]: https://github.com/jedarden/ccdash/releases/tag/v0.1.4
[0.1.3]: https://github.com/jedarden/ccdash/releases/tag/v0.1.3
[0.1.2]: https://github.com/jedarden/ccdash/releases/tag/v0.1.2
[0.1.1]: https://github.com/jedarden/ccdash/releases/tag/v0.1.1
[0.1.0]: https://github.com/jedarden/ccdash/releases/tag/v0.1.0
