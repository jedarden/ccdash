# GitHub Release Instructions for v0.1.4

## ✅ Completed Steps

1. **Code Changes Committed and Pushed**
   - Fixed panel width calculations
   - Added version display
   - Improved tmux detection
   - Updated documentation
   - Commit: 56e9b21

2. **Git Tag Created and Pushed**
   - Tag: v0.1.4
   - Successfully pushed to origin

3. **Release Binary Built**
   - Clean build completed
   - Binary location: `releases/v0.1.4/ccdash-linux-amd64`
   - Size: 3.5M
   - Stripped symbols for smaller size

4. **SHA256 Checksum Generated**
   - Checksum: `626487d87a2117c5f96454ad2a2f8ed1ed4fee97ce8051810c2fb1b6a7db844e`
   - File: `releases/v0.1.4/ccdash-linux-amd64.sha256`

5. **Release Notes Created**
   - Comprehensive release notes with installation instructions
   - File: `releases/v0.1.4/RELEASE_NOTES.md`

## 🚀 Next Step: Create GitHub Release

You need to authenticate with GitHub CLI and create the release.

### Option 1: Using GitHub CLI (Recommended)

```bash
# Authenticate (one-time setup)
gh auth login

# Create the release
cd /workspaces/test-agor/ccdash
gh release create v0.1.4 \
  --title "v0.1.4 - Fix Panel Width Calculations" \
  --notes-file releases/v0.1.4/RELEASE_NOTES.md \
  releases/v0.1.4/ccdash-linux-amd64 \
  releases/v0.1.4/ccdash-linux-amd64.sha256
```

### Option 2: Manual GitHub Web Interface

1. Go to: https://github.com/jedarden/ccdash/releases/new
2. Choose tag: `v0.1.4`
3. Set title: `v0.1.4 - Fix Panel Width Calculations`
4. Copy content from `releases/v0.1.4/RELEASE_NOTES.md` into description
5. Upload files:
   - `releases/v0.1.4/ccdash-linux-amd64`
   - `releases/v0.1.4/ccdash-linux-amd64.sha256`
6. Click "Publish release"

## 📦 Release Assets

The following files are ready for upload:

```
releases/v0.1.4/
├── ccdash-linux-amd64           (3.5M) - Standalone binary
├── ccdash-linux-amd64.sha256    (85B)  - SHA256 checksum
└── RELEASE_NOTES.md             (5.4K) - Release documentation
```

### Binary Details

**Filename:** `ccdash-linux-amd64`
**Size:** 3.5M
**SHA256:** `626487d87a2117c5f96454ad2a2f8ed1ed4fee97ce8051810c2fb1b6a7db844e`

**Platform:** Linux x86_64
**Go Version:** Built with Go 1.21+
**Static Binary:** Yes (no external dependencies)

### Verification Command

Users can verify the download with:
```bash
sha256sum -c ccdash-linux-amd64.sha256
```

Expected output: `ccdash-linux-amd64: OK`

## 📝 Release Highlights

Key features to mention in release announcement:

- 🐛 Fixed panel width calculations for proper rendering
- ✨ Version display in status bar
- 🎯 Enhanced tmux session detection with unified-dashboard patterns
- 📚 Comprehensive documentation updates
- 🔒 SHA256 checksum for binary verification
- 📦 Standalone binary - no dependencies required

## 🔗 Quick Links

- Repository: https://github.com/jedarden/ccdash
- Releases: https://github.com/jedarden/ccdash/releases
- Issues: https://github.com/jedarden/ccdash/issues
- Tag: https://github.com/jedarden/ccdash/releases/tag/v0.1.4

## ✅ Release Checklist

- [x] Code changes committed
- [x] Git tag created and pushed
- [x] Binary built and tested
- [x] SHA256 checksum generated
- [x] Release notes written
- [x] CHANGELOG.md updated
- [x] README.md updated
- [ ] GitHub release created (awaiting authentication)
- [ ] Release announcement (optional)

---

**Note:** Once the GitHub release is created, users will be able to download the binary directly from:
```
https://github.com/jedarden/ccdash/releases/download/v0.1.4/ccdash-linux-amd64
https://github.com/jedarden/ccdash/releases/download/v0.1.4/ccdash-linux-amd64.sha256
```
