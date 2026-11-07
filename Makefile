# Fiskus — bežné príkazy. Na Windows funguje cez Git Bash alebo WSL.

WEB_DIST := backend/internal/web/dist
VERSION  ?= dev

.PHONY: test test-go test-web test-browser web build browser demo dev-api dev-web snimky

test: test-go test-web

test-go:
	cd backend && go vet ./... && go test -race ./...
	cd backend && go test -tags sqlite ./...

test-web:
	cd frontend && npm test

# úložisko SQLite v prehliadači (WebAssembly) musí prejsť tými istými testami
test-browser:
	cd backend && GOOS=js GOARCH=wasm go test -tags sqlite \
		-exec $(CURDIR)/frontend/scripts/go-wasm-exec.sh ./internal/store/sqlite/

# zostaví frontend priamo do Go balíka web (vloží sa do binárky)
web:
	cd frontend && npx vite build --outDir ../$(WEB_DIST) --emptyOutDir
	touch $(WEB_DIST)/.gitkeep

build: web
	cd backend && CGO_ENABLED=0 go build -tags sqlite \
		-ldflags "-s -w -X main.version=$(VERSION)" -o ../bin/fiskus ./cmd/fiskus
	cd backend && CGO_ENABLED=0 go build -ldflags "-s -w" -o ../bin/fiskus-mcp ./cmd/fiskus-mcp

# Fiškus v prehliadači: statický web v frontend/dist-browser (nahrá sa na
# ľubovoľný hosting s HTTPS, do koreňa domény, napr. app.fiskus.online).
# Program ide skomprimovaný (fiskus.wasm.gz, rozbalí ho prehliadač) — nezáleží
# na tom, či hosting komprimuje, a súbor je pod limitom 25 MiB Cloudflare Pages.
browser:
	cd backend && GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -tags sqlite -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" -o ../frontend/src/browser/fiskus.wasm ./cmd/fiskus-web
	gzip -9 -n -c frontend/src/browser/fiskus.wasm > frontend/src/browser/fiskus.wasm.gz
	cp "$$(cd backend && go env GOROOT)/lib/wasm/wasm_exec.js" frontend/src/browser/wasm_exec.js
	cd frontend && npx vite build --mode browser --emptyOutDir

# ukážka s demo dátami: http://127.0.0.1:8080
demo: web
	cd backend && go run ./cmd/fiskus -demo

# vývoj: v jednom termináli API, v druhom frontend s okamžitým obnovovaním
dev-api:
	cd backend && go run ./cmd/fiskus -demo

dev-web:
	cd frontend && npm run dev

# Snímky obrazovky do návodu (docs/obrazky) a na web (site/obrazky) z ukážkových
# dát. Potrebuje Playwright (npx playwright install chromium) a Python s Pillow.
snimky: web
	cd backend && go build -o ../.snimky-fiskus ./cmd/fiskus
	./.snimky-fiskus -demo -addr 127.0.0.1:8199 -rpo-url "" -ecb-url "" & echo $$! > .snimky.pid; \
	sleep 3; BASE=http://127.0.0.1:8199 node tools/snimky/snimky.cjs; s=$$?; \
	kill $$(cat .snimky.pid); rm -f .snimky.pid .snimky-fiskus; \
	[ $$s -eq 0 ] && python3 tools/snimky/webp.py; exit $$s
