.PHONY: isureal preset-images clean

isureal:
	go build -buildvcs=false -o isureal cmd/isureal/main.go

preset-images:
	go build -buildvcs=false -o preset-images cmd/preset-images/main.go

clean:
	rm -f isureal preset-images
