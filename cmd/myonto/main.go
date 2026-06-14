// myonto 是个人本体论管理助手。
//
// 用法见各子命令的 --help。所有命令操作当前目录（或父目录）下的
// ontology.ttl 文件，类似 git 的工作流。
package main

import (
	"fmt"
	"os"

	"github.com/walker/myonto/internal/cli"
)

func main() {
	if err := cli.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "myonto: %v\n", err)
		os.Exit(1)
	}
}
