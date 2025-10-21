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
	@echo "  4. GitHub Actions automatically:"
	@echo "     - Updates Homebrew formula with correct SHA256"
	@echo "     - Builds Windows binary"
	@echo "     - Creates GitHub release"

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
	echo "🎉 Tag $$NEW_VERSION pushed!"; \
	echo ""; \
	echo "⏳ GitHub Actions will now:"; \
	echo "   1. Update Homebrew formula automatically"; \
	echo "   2. Build Windows binary"; \
	echo "   3. Create GitHub release"; \
	echo ""; \
	echo "📦 Check progress:"; \
	echo "   https://github.com/hnrqer/transcriber-pro/actions"; \
	echo ""; \
	echo "✅ Once complete, users can upgrade with:"; \
	echo "   brew update && brew upgrade transcriber-pro"
