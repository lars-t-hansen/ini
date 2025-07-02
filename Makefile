.PHONY: default test

default:
	@echo "Pick an explicit target"

test:
	go test

README.md: ini.go
	echo "# ini" > README.md
	echo "" >> README.md
	go doc | awk '/^func / { exit } { print }' | tail -n +3 >> README.md
