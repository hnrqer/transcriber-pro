.PHONY: help release-patch release-minor release-major update-formula

help:
	@echo "Transcriber Pro - Release Automation"
	@echo ""
	@echo "Usage:"
	@echo "  make release-patch   Bump patch version (2.0.8 -> 2.0.9)"
	@echo "  make release-minor   Bump minor version (2.0.8 -> 2.1.0)"
	@echo "  make release-major   Bump major version (2.0.8 -> 3.0.0)"
	@echo ""
	@echo "The release process will:"
	@echo "  1. Get current version from latest git tag"
	@echo "  2. Bump version according to type"
	@echo "  3. Create and push new git tag"
	@echo "  4. Download source tarball (available immediately)"
	@echo "  5. Calculate SHA256"
	@echo "  6. Update Homebrew formula"
	@echo "  7. Commit and push formula"
	@echo ""
	@echo "Note: Windows build runs in background (not required for formula)"

release-patch:
	@$(MAKE) release BUMP=patch

release-minor:
	@$(MAKE) release BUMP=minor

release-major:
	@$(MAKE) release BUMP=major

release:
	@echo "🚀 Starting $(BUMP) release..."
	@echo ""

	@# Get current version
	@CURRENT_VERSION=$$(git describe --tags --abbrev=0 2>/dev/null || echo "v1.0.0"); \
	echo "📌 Current version: $$CURRENT_VERSION"; \
	\
	VERSION=$$(echo $$CURRENT_VERSION | sed 's/^v//'); \
	MAJOR=$$(echo $$VERSION | cut -d. -f1); \
	MINOR=$$(echo $$VERSION | cut -d. -f2); \
	PATCH=$$(echo $$VERSION | cut -d. -f3); \
	\
	if [ "$(BUMP)" = "major" ]; then \
		MAJOR=$$((MAJOR + 1)); MINOR=0; PATCH=0; \
	elif [ "$(BUMP)" = "minor" ]; then \
		MINOR=$$((MINOR + 1)); PATCH=0; \
	else \
		PATCH=$$((PATCH + 1)); \
	fi; \
	\
	NEW_VERSION="v$$MAJOR.$$MINOR.$$PATCH"; \
	echo "✨ New version: $$NEW_VERSION"; \
	echo ""; \
	\
	read -p "Continue? [y/N] " -n 1 -r; \
	echo; \
	if [[ ! $$REPLY =~ ^[Yy]$$ ]]; then \
		echo "❌ Cancelled"; \
		exit 1; \
	fi; \
	\
	echo "📝 Creating git tag $$NEW_VERSION..."; \
	git tag -a $$NEW_VERSION -m "Release $$NEW_VERSION"; \
	git push origin $$NEW_VERSION; \
	echo ""; \
	\
	echo "⏳ Waiting 5 seconds for GitHub to create tarball..."; \
	sleep 5; \
	echo ""; \
	\
	echo "📥 Downloading release tarball..."; \
	curl -sL "https://github.com/hnrqer/transcriber-pro/archive/refs/tags/$$NEW_VERSION.tar.gz" -o /tmp/transcriber-pro-$$NEW_VERSION.tar.gz; \
	echo ""; \
	\
	echo "🔐 Calculating SHA256..."; \
	SHA256=$$(shasum -a 256 /tmp/transcriber-pro-$$NEW_VERSION.tar.gz | cut -d' ' -f1); \
	echo "   SHA256: $$SHA256"; \
	echo ""; \
	\
	echo "📝 Updating Homebrew formula..."; \
	sed -i '' "s|archive/refs/tags/v.*\.tar\.gz|archive/refs/tags/$$NEW_VERSION.tar.gz|g" Formula/transcriber-pro.rb; \
	sed -i '' "s|sha256 \".*\"|sha256 \"$$SHA256\"|g" Formula/transcriber-pro.rb; \
	echo ""; \
	\
	echo "✅ Formula updated!"; \
	echo ""; \
	echo "📝 Committing formula update..."; \
	git add Formula/transcriber-pro.rb; \
	git commit -m "Update Homebrew formula to $$NEW_VERSION"; \
	git push; \
	echo ""; \
	\
	rm /tmp/transcriber-pro-$$NEW_VERSION.tar.gz; \
	\
	echo ""; \
	echo "🎉 Release $$NEW_VERSION complete!"; \
	echo ""; \
	echo "📦 Users can now install with:"; \
	echo "   brew upgrade transcriber-pro"; \
	echo ""; \
	echo "ℹ️  Note: Windows build is still running in background"; \
	echo "   Check: https://github.com/hnrqer/transcriber-pro/actions"
