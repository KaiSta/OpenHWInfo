# OpenHWInfo Makefile
# Usage: make build | make run | make dev | make test | make install | make package | make clean
#
# Build tags explained:
#   production  -> builds the real app (without it, Wails compiles a no-op stub)
#   webkit2_41  -> use webkit2gtk-4.1 (this system has 4.1; drop it for 4.0 systems)

BINARY := OpenHWInfo
TAGS    := production,webkit2_41
DEV_TAGS := dev,webkit2_41

# Packaging (make package / make install). The version lives in ./VERSION, which
# the release workflow checks the tag against, so a release is bumped by editing
# that file. Override it for a one-off package:
#   make package VERSION=1.0.0
# Lowercase id used for the desktop entry, icon and deb package name.
APP      := openhwinfo
VERSION  ?= $(shell cat VERSION)
ARCH     := $(shell dpkg --print-architecture)
# Maintainer recorded in the .deb control file. Override with your real address
# when publishing: make package MAINTAINER="Your Name <you@example.org>"
MAINTAINER ?= KaiSta <kai@example.com>
PKGDIR   := dist/$(APP)-package
DEBNAME  := $(APP)_$(VERSION)_$(ARCH).deb

# hicolor icon install is factored so install/package stay in sync.
ICON_SIZES := 16 32 48 64 128 192 256 512
ICON_FILES := 16:icons/icon-32.png 32:icons/icon-48.png 48:icons/icon-64.png 64:icons/icon-64.png 128:icons/icon-128.png 192:icons/icon-192.png 256:icons/icon-512.png 512:icons/icon-512.png
define install-icons
	for pair in $(ICON_FILES); do \
		size=$${pair%%:*}; file=$${pair#*:}; \
		install -Dm0644 "$$file" "$(1)/usr/share/icons/hicolor/$${size}x$${size}/apps/openhwinfo.png"; \
	done
endef

.PHONY: build run dev test lint map hooks install package clean

## Build the native binary (production)
build:
	go vet ./...
	go build -tags=$(TAGS) -o $(BINARY) .

## Run the app in production mode
run: build
	./$(BINARY)

## Run in development mode (hot-reload, serves frontend from disk)
dev:
	go run -tags=$(DEV_TAGS) .

## Run all tests
test:
	go test ./...

lint:
	go vet ./...

# Regenerate the code-map indexes under .claude/skills/code-map/docs/ from the
# source. They are committed, so that every agent finds them without building
# first, and `go test ./...` fails when one is stale -- so run this (and commit
# the result) whenever a symbol is added, renamed or removed.
map:
	go test . -run TestAgentDocsIndexesAreCurrent -agentdocs.update

# Install the repository's git hooks (see .githooks/). Git only reads hooks from
# $GIT_DIR/hooks, which is not versioned, so this points core.hooksPath at the
# tracked directory instead. It writes to this clone's repository config only, and
# is safe to re-run.
hooks:
	git config core.hooksPath .githooks
	@echo "Hooks installed: pre-commit gates gofmt and the generated code map,"
	@echo "commit-msg checks the subject line. CI remains the authority."

# Install the desktop entry + icon so Cinnamon/GNOME show our logo in the
# panel/taskbar (they match a window's WM_CLASS to a .desktop file, not the
# GTK window icon). Idempotent; safe to re-run.
install: build
	@echo "Installing desktop entry and icons for $(APP)..."
	mkdir -p ~/.local/share/applications
	for s in $(ICON_SIZES); do mkdir -p ~/.local/share/icons/hicolor/$${s}x$${s}/apps; done
	for pair in $(ICON_FILES); do size=$${pair%%:*}; file=$${pair#*:}; cp "$$file" ~/.local/share/icons/hicolor/$${size}x$${size}/apps/openhwinfo.png; done
	sed "s|@EXEC@|$(CURDIR)/$(BINARY)|" openhwinfo.desktop > ~/.local/share/applications/openhwinfo.desktop
	gtk-update-icon-cache ~/.local/share/icons/hicolor >/dev/null 2>&1 || true
	update-desktop-database ~/.local/share/applications >/dev/null 2>&1 || true
	@echo "Done. Restart the app (or log out/in) to see the new panel icon."

# Build a .deb installer into dist/ (binary + desktop entry + hicolor icons,
# same layout as `install` but system-wide). Install with: sudo dpkg -i dist/$(DEBNAME)
package: build
	rm -rf $(PKGDIR) dist/$(DEBNAME)
	mkdir -p $(PKGDIR)/DEBIAN \
	         $(PKGDIR)/usr/bin \
	         $(PKGDIR)/usr/share/applications
	for s in $(ICON_SIZES); do mkdir -p $(PKGDIR)/usr/share/icons/hicolor/$${s}x$${s}/apps; done
	cp $(BINARY) $(PKGDIR)/usr/bin/$(APP)
	sed "s|@EXEC@|/usr/bin/$(APP)|" openhwinfo.desktop > $(PKGDIR)/usr/share/applications/openhwinfo.desktop
	for pair in $(ICON_FILES); do size=$${pair%%:*}; file=$${pair#*:}; cp "$$file" $(PKGDIR)/usr/share/icons/hicolor/$${size}x$${size}/apps/openhwinfo.png; done
	printf 'Package: %s\nVersion: %s\nArchitecture: %s\nMaintainer: %s\nDepends: libwebkit2gtk-4.1-0, libgtk-3-0t64 | libgtk-3-0\nDescription: OpenHWInfo — live CPU, GPU & mainboard hardware monitor (Go + Wails v2)\n Displays real-time CPU, GPU and mainboard metrics such as temperature,\n clock, usage, power draw, VRAM and board fan speeds.\n' \
	  "$(APP)" "$(VERSION)" "$(ARCH)" "$(MAINTAINER)" > $(PKGDIR)/DEBIAN/control
	chmod 0755 $(PKGDIR)/usr/bin/$(APP)
	dpkg-deb -b $(PKGDIR) dist/$(DEBNAME)
	@echo "Created dist/$(DEBNAME). Install with: sudo dpkg -i dist/$(DEBNAME)"

## Clean build artifacts
clean:
	rm -f $(BINARY)
	rm -rf dist/