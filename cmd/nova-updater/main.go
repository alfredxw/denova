package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"nova/internal/buildinfo"
	"nova/internal/update"
)

func main() {
	var (
		manifestPath string
		showVersion  bool
	)
	flag.StringVar(&manifestPath, "manifest", "", "待应用更新清单路径")
	flag.BoolVar(&showVersion, "version", false, "输出版本号")
	flag.Parse()

	if showVersion {
		fmt.Println(buildinfo.Version)
		return
	}
	if manifestPath == "" {
		fmt.Fprintln(os.Stderr, "缺少 --manifest 参数")
		os.Exit(2)
	}
	if err := update.RunUpdater(context.Background(), manifestPath, update.UpdaterOptions{}); err != nil {
		fmt.Fprintf(os.Stderr, "应用更新失败: %v\n", err)
		os.Exit(1)
	}
}
