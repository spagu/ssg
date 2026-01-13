#!/bin/bash
# Script to download external images and update markdown files
# Handles both /uploads/YYYY/MM/ and /uploads/sites/N/YYYY/MM/ patterns

set -e

CONTENT_DIR="content/krowy.net.2026-01-13110345"
MEDIA_DIR="$CONTENT_DIR/media"

# Create media directory if not exists
mkdir -p "$MEDIA_DIR"

# Extract unique URLs - both patterns
URLS=$(grep -rohE 'https?://(www\.)?krowy\.net/wp-content/uploads/(sites/[0-9]+/)?[0-9]{4}/[0-9]{2}/[^"'\''<>\s]+' "$CONTENT_DIR" | sort -u)

echo "Found $(echo "$URLS" | wc -l) unique URLs to process"

# Download each URL
for URL in $URLS; do
    # Skip incomplete URLs (not ending with image extension)
    if [[ ! "$URL" =~ \.(jpg|jpeg|png|gif|webp)$ ]]; then
        echo "Skipping incomplete URL: $URL"
        continue
    fi
    
    # Generate local filename from URL
    FILENAME=$(basename "$URL")
    
    # Check if file already exists
    if [[ -f "$MEDIA_DIR/$FILENAME" ]]; then
        echo "Already exists: $FILENAME"
    else
        echo "Downloading: $URL -> $FILENAME"
        wget -q -O "$MEDIA_DIR/$FILENAME" "$URL" || echo "Failed to download: $URL"
    fi
done

echo ""
echo "Downloads complete. Now updating markdown files..."

# Update markdown files - replace URLs with local paths
# Pattern 1: http://www.krowy.net/wp-content/uploads/YYYY/MM/filename.ext -> media/filename.ext
# Pattern 2: http://krowy.net/wp-content/uploads/sites/N/YYYY/MM/filename.ext -> media/filename.ext
find "$CONTENT_DIR" -name "*.md" -type f | while read -r MDFILE; do
    if grep -q 'krowy\.net/wp-content/uploads/' "$MDFILE"; then
        echo "Updating: $MDFILE"
        # Replace both patterns
        sed -i -E 's|https?://(www\.)?krowy\.net/wp-content/uploads/(sites/[0-9]+/)?[0-9]{4}/[0-9]{2}/([^"'\''<>\s]+)|media/\3|g' "$MDFILE"
    fi
done

echo "Done!"
