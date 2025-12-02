#!/bin/bash
set -e

# Script to extract deployer-specific files to a separate directory
# This creates a wrapper structure that can be overlaid on the flyctl project

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTPUT_DIR="${1:-$SCRIPT_DIR/../flyctl-deployer-overlay}"

echo "Extracting deployer files to: $OUTPUT_DIR"
echo ""

# Create output directory
mkdir -p "$OUTPUT_DIR"

# List of files to extract (relative to repo root)
FILES=(
    "deployer.Dockerfile"
    "deployer.Dockerfile.dockerignore"
    "deploy.rb"
    "deploy/common.rb"
    ".github/workflows/deployer-tests.yml"
    "scripts/deployer-tests.sh"
    "internal/command/launch/sessions.go"
    "scanner/ruby_test.go"
    "test/deployer/deployer_test.go"
    "test/testlib/deployer.go"
)

# Test fixture directories to copy
FIXTURE_DIRS=(
    "test/fixtures/bun-basic"
    "test/fixtures/deno-no-config"
    "test/fixtures/deploy-node-monorepo"
    "test/fixtures/deploy-node-no-dockerfile"
    "test/fixtures/deploy-node-yarn"
    "test/fixtures/deploy-node"
    "test/fixtures/deploy-phoenix-sqlite-custom-tool-versions"
    "test/fixtures/deploy-phoenix-sqlite"
    "test/fixtures/deploy-rails-7.0"
    "test/fixtures/deploy-rails-7.2"
    "test/fixtures/deploy-rails-8"
    "test/fixtures/django-basic"
    "test/fixtures/go-no-go-sum"
    "test/fixtures/static"
)

echo "Copying individual files..."
for file in "${FILES[@]}"; do
    if [ -f "$SCRIPT_DIR/$file" ]; then
        # Create parent directory structure
        target_dir="$OUTPUT_DIR/$(dirname "$file")"
        mkdir -p "$target_dir"

        # Copy file
        cp "$SCRIPT_DIR/$file" "$OUTPUT_DIR/$file"
        echo "  ✓ $file"
    else
        echo "  ⚠ $file (not found)"
    fi
done

echo ""
echo "Copying test fixture directories..."
for dir in "${FIXTURE_DIRS[@]}"; do
    if [ -d "$SCRIPT_DIR/$dir" ]; then
        # Create parent directory structure
        target_parent="$OUTPUT_DIR/$(dirname "$dir")"
        mkdir -p "$target_parent"

        # Copy entire directory
        cp -r "$SCRIPT_DIR/$dir" "$OUTPUT_DIR/$dir"
        echo "  ✓ $dir"
    else
        echo "  ⚠ $dir (not found)"
    fi
done

# Create a README explaining the overlay structure
cat > "$OUTPUT_DIR/README.md" << 'EOF'
# Flyctl Deployer Overlay

This directory contains deployer-specific files that extend the base flyctl project.

## Structure

This overlay includes:

### Core Deployer Files
- `deployer.Dockerfile` - Multi-runtime Docker image supporting Ruby, Node, Python, Elixir, PHP, Bun, Go, Deno
- `deploy.rb` - Main orchestration script for deployment workflow
- `deploy/common.rb` - Shared utilities for deployment operations

### Integration Files
- `internal/command/launch/sessions.go` - Session management for launch workflow
- `.github/workflows/deployer-tests.yml` - CI workflow for deployer tests
- `scripts/deployer-tests.sh` - Test runner for deployer functionality

### Test Infrastructure
- `test/deployer/deployer_test.go` - Integration tests
- `test/testlib/deployer.go` - Test utilities
- `scanner/ruby_test.go` - Ruby scanner tests
- `test/fixtures/` - Comprehensive test fixtures for:
  - Bun applications
  - Deno applications
  - Node.js (various configurations)
  - Phoenix/Elixir (multiple versions)
  - Rails (7.0, 7.2, 8)
  - Django
  - Go
  - Static sites

## Usage

To apply this overlay to a flyctl repository:

```bash
# Option 1: Copy files directly
cp -r flyctl-deployer-overlay/* /path/to/flyctl/

# Option 2: Use rsync to merge
rsync -av flyctl-deployer-overlay/ /path/to/flyctl/

# Option 3: Create symbolic links (for development)
cd /path/to/flyctl
ln -s /path/to/flyctl-deployer-overlay/deployer.Dockerfile .
ln -s /path/to/flyctl-deployer-overlay/deploy.rb .
# ... etc
```

## Development Workflow

This structure allows you to:
1. Maintain deployer functionality separately from the main flyctl codebase
2. Version control the overlay independently
3. Apply/update the overlay across different flyctl branches
4. Test deployer functionality in isolation

## Integration with Flyctl

The deployer extends flyctl's launch workflow by:
- Adding session-based customization (`sessions.go`)
- Providing multi-runtime Docker builds (`deployer.Dockerfile`)
- Orchestrating complex deployment scenarios (`deploy.rb`)
- Supporting manifest-based plan generation

## Testing

Run deployer tests:
```bash
./scripts/deployer-tests.sh
# or
go test ./test/deployer/...
```

## Maintenance

When updating flyctl:
1. Pull latest changes to your flyctl repository
2. Reapply this overlay
3. Resolve any conflicts in integration points
4. Run tests to verify compatibility
EOF

# Create an install script
cat > "$OUTPUT_DIR/install.sh" << 'EOF'
#!/bin/bash
set -e

# Installation script for deployer overlay
TARGET_DIR="${1:-.}"

if [ ! -d "$TARGET_DIR" ]; then
    echo "Error: Target directory '$TARGET_DIR' does not exist"
    exit 1
fi

if [ ! -f "$TARGET_DIR/go.mod" ] || ! grep -q "github.com/superfly/flyctl" "$TARGET_DIR/go.mod" 2>/dev/null; then
    echo "Warning: Target directory doesn't appear to be a flyctl repository"
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "Installing deployer overlay to: $TARGET_DIR"
echo ""

# Copy all files except install.sh and README.md
rsync -av --exclude='install.sh' --exclude='README.md' --exclude='.git' "$SCRIPT_DIR/" "$TARGET_DIR/"

echo ""
echo "✓ Deployer overlay installed successfully!"
echo ""
echo "Next steps:"
echo "  1. Review the changes: cd $TARGET_DIR && git status"
echo "  2. Build: go build"
echo "  3. Run tests: ./scripts/deployer-tests.sh"
EOF

chmod +x "$OUTPUT_DIR/install.sh"

# Create a manifest file listing all extracted files
echo "Creating manifest..."
cat > "$OUTPUT_DIR/MANIFEST.txt" << EOF
# Deployer Overlay Manifest
# Generated: $(date)
# Source: $SCRIPT_DIR

## Individual Files
EOF

for file in "${FILES[@]}"; do
    if [ -f "$SCRIPT_DIR/$file" ]; then
        echo "$file" >> "$OUTPUT_DIR/MANIFEST.txt"
    fi
done

cat >> "$OUTPUT_DIR/MANIFEST.txt" << EOF

## Test Fixture Directories
EOF

for dir in "${FIXTURE_DIRS[@]}"; do
    if [ -d "$SCRIPT_DIR/$dir" ]; then
        echo "$dir/" >> "$OUTPUT_DIR/MANIFEST.txt"
    fi
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✓ Extraction complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Output directory: $OUTPUT_DIR"
echo ""
echo "Contents:"
du -sh "$OUTPUT_DIR"
find "$OUTPUT_DIR" -type f | wc -l | xargs echo "  Files:"
find "$OUTPUT_DIR" -type d | wc -l | xargs echo "  Directories:"
echo ""
echo "To install this overlay on another flyctl repository:"
echo "  cd /path/to/flyctl"
echo "  $OUTPUT_DIR/install.sh ."
echo ""
echo "Documentation: $OUTPUT_DIR/README.md"
echo "Manifest: $OUTPUT_DIR/MANIFEST.txt"
