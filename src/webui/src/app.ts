/**
 * NetProxy-Magisk WebUI
 * 模块化架构 - 主入口文件
 */

import "mdui/mdui.css";
import "mdui";
import { UI } from "./ui/ui-core.js";

// Declare KernelSU global type
declare global {
  interface Window {
    ksu?: {
      exec: (command: string) => {
        errno: number;
        stdout: string;
        stderr: string;
      };
      spawn: (command: string, args: string[], options?: object) => object;
      toast: (message: string) => void;
      fullScreen: (enable: boolean) => void;
    };
  }
}

/**
 * 等待 KernelSU 环境准备好再初始化
 */
function initializeApp(): void {
  // 检查 ksu 对象是否可用
  if (typeof window.ksu !== "undefined") {
    new UI();
  } else {
    setTimeout(() => {
      new UI();
    }, 500);
  }
}

// 初始化应用
if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", initializeApp);
} else {
  initializeApp();
}
