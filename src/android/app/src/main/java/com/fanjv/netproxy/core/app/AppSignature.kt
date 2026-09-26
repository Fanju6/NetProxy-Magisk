package com.fanjv.netproxy.core.app

import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import java.security.MessageDigest

private const val GOOGLE_PLAY_APP_SIGNING_SHA256 =
    "24:54:65:F5:FF:D9:55:17:1A:AE:EA:FB:AC:AF:25:75:C0:42:07:8B:4E:F6:99:15:99:32:6A:53:D0:0F:C7:C1"
private const val HEX_DIGITS = "0123456789ABCDEF"

@Suppress("DEPRECATION")
internal fun isSignedWithGooglePlayKey(context: Context): Boolean = runCatching {
    val packageManager = context.packageManager
    val flags = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
        PackageManager.GET_SIGNING_CERTIFICATES
    } else {
        PackageManager.GET_SIGNATURES
    }
    val packageInfo = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
        packageManager.getPackageInfo(
            context.packageName,
            PackageManager.PackageInfoFlags.of(flags.toLong())
        )
    } else {
        packageManager.getPackageInfo(context.packageName, flags)
    }
    val signers = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
        val signingInfo = packageInfo.signingInfo ?: return@runCatching false
        // 证书轮换后，签名历史用于识别仍由 Play 密钥链签发的版本。
        if (signingInfo.hasMultipleSigners()) {
            signingInfo.apkContentsSigners
        } else {
            signingInfo.signingCertificateHistory
        }
    } else {
        packageInfo.signatures
    }
    signers.orEmpty().any { signature ->
        matchesGooglePlayFingerprint(sha256Fingerprint(signature.toByteArray()))
    }
}.getOrDefault(false)

internal fun sha256Fingerprint(certificate: ByteArray): String {
    val digest = MessageDigest.getInstance("SHA-256").digest(certificate)
    return buildString(digest.size * 3 - 1) {
        digest.forEachIndexed { index, byte ->
            if (index > 0) append(':')
            val value = byte.toInt() and 0xFF
            append(HEX_DIGITS[value ushr 4])
            append(HEX_DIGITS[value and 0x0F])
        }
    }
}

internal fun matchesGooglePlayFingerprint(fingerprint: String): Boolean =
    fingerprint.filterNot { it == ':' || it.isWhitespace() }
        .equals(GOOGLE_PLAY_APP_SIGNING_SHA256.filterNot { it == ':' }, ignoreCase = true)
