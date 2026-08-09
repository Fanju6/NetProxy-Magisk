package com.fanjv.netproxy.feature.kernel.presentation

/** sing-box 配置工作台中的文件分类。 */
enum class SingBoxDocumentCategory {
    Config,
    Source,
    Runtime,
}

/** 允许由管理器读取的 sing-box 配置文档。 */
data class SingBoxDocument(
    val id: String,
    val filename: String,
    val category: SingBoxDocumentCategory,
    val editable: Boolean,
)

/** sing-box 配置文件保存结果。 */
data class SingBoxDocumentSaveResult(
    val success: Boolean,
    val errorMessage: String? = null,
    val restored: Boolean = false,
)


