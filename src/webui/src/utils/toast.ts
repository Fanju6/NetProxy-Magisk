import { snackbar } from "mdui";

/**
 * 基于 mdui snackbar 的增强 toast 函数
 * @param {string} msg - 要显示的消息
 * @param {boolean} closeable - 是否允许手动关闭
 */
export function toast(msg: string, closeable: boolean = false): void {
  snackbar({
    message: msg,
    closeable: closeable,
    autoCloseDelay: closeable ? 0 : 3000,
    placement: "bottom",
  } as any);
}
