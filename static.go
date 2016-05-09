package main

import (
	"github.com/elazarl/go-bindata-assetfs"
)

//go:generate go-bindata-assetfs -pkg=main ./static/...
//go:generate go fmt .

func AssetFS() *assetfs.AssetFS {
	return assetFS()
}
