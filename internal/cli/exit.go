package cli

import "os"

// exitWith 请求程序以指定 code 退出。
//
// 命令在需要非零退出码时（如 path 未找到、check --strict 有 error）
// 调用 exitWith(code) 而非 os.Exit(code)。
//
// 在生产入口（Run）下，exitWith = os.Exit，直接终止。
// 在测试入口（RunArgs）下，exitWith 被替换为 panic(exitSignal{code})，
// 由 RunArgs 的 recover 捕获，使测试进程不会被杀掉。
var exitWith = os.Exit
