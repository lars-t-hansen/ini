.PHONY: default test

default:
	@echo "Pick an explicit target"

test:
	if [[ `gofmt -l *.go | wc -l` != "0" ]]; then echo "Bad formatting"; exit 1; fi
	go test

README.md: ini.go
	echo "# ini" > README.md
	echo "" >> README.md
	go doc | awk '/^func / { exit } { print }' | tail -n +3 >> README.md
