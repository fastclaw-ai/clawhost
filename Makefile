.PHONY: dev build admin

admin:
	cd web/admin && npm install && npx next build --webpack
	touch web/admin/out/.gitkeep

build: admin
	go build -o clawhost .

dev:
	air -c .air.toml server