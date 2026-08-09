.section .text
.global _start
.type _start, %function

// 入口只做路径拼接和 execve，不依赖 libc 或 Shell。
_start:
    mov     x19, sp
    ldr     x20, [x19]                  // argc
    add     x21, x19, #8                // argv
    ldr     x22, [x21]                  // argv[0]
    cbz     x22, fallback_path

    // 找到 argv[0] 中最后一个斜杠，保留模块根目录前缀。
    mov     x23, x22
    mov     x24, xzr                    // 最后一个斜杠后的地址
scan_argv0:
    ldrb    w0, [x23], #1
    cbz     w0, argv0_scanned
    cmp     w0, #'/'
    csel    x24, x23, x24, eq
    b       scan_argv0

argv0_scanned:
    cbz     x24, fallback_path

    adrp    x25, exec_path
    add     x25, x25, :lo12:exec_path
    mov     x26, x22
copy_prefix:
    cmp     x26, x24
    b.eq    copy_suffix_start
    ldrb    w0, [x26], #1
    strb    w0, [x25], #1
    b       copy_prefix

copy_suffix_start:
    adrp    x27, suffix
    add     x27, x27, :lo12:suffix
copy_suffix:
    ldrb    w0, [x27], #1
    strb    w0, [x25], #1
    cbnz    w0, copy_suffix
    b       exec_target

// 直接调用模块根目录时 argv[0] 理论上带路径；无路径时使用固定模块路径。
fallback_path:
    adrp    x25, exec_path
    add     x25, x25, :lo12:exec_path
    adrp    x27, fallback
    add     x27, x27, :lo12:fallback
copy_fallback:
    ldrb    w0, [x27], #1
    strb    w0, [x25], #1
    cbnz    w0, copy_fallback

exec_target:
    adrp    x0, exec_path
    add     x0, x0, :lo12:exec_path
    mov     x1, x21                     // 原样透传 argv
    add     x2, x21, x20, lsl #3
    add     x2, x2, #8                   // envp = argv + argc + 1
    mov     x8, #221                     // AArch64 execve
    svc     #0

    // execve 失败时以 126 退出，避免回到调用方继续执行。
    mov     x0, #126
    mov     x8, #93                      // AArch64 exit
    svc     #0
    b       .

.section .rodata
.align  3
suffix:
    .asciz  "bin/netproxyctl"
fallback:
    .asciz  "/data/adb/modules/netproxy/bin/netproxyctl"

.section .bss
.align  4
exec_path:
    .skip   4096
