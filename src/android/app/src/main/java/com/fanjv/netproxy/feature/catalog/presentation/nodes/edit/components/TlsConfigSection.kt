package com.fanjv.netproxy.feature.catalog.presentation.nodes.edit.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import com.fanjv.netproxy.R
import top.yukonga.miuix.kmp.basic.Card
import top.yukonga.miuix.kmp.basic.SmallTitle
import top.yukonga.miuix.kmp.preference.OverlayDropdownPreference
import top.yukonga.miuix.kmp.preference.SwitchPreference

/** TLS、uTLS、Reality 和 ECH 配置区。 */
@Composable
internal fun TlsConfigSection(
    enabled: Boolean,
    serverName: String,
    insecure: Boolean,
    disableSni: Boolean,
    alpn: String,
    fingerprint: String,
    realityEnabled: Boolean,
    realityPublicKey: String,
    realityShortId: String,
    echEnabled: Boolean,
    echConfig: String,
    echQueryServerName: String,
    alpnLabel: String,
    utlsFingerprintLabel: String,
    realityPublicKeyLabel: String,
    realityShortIdLabel: String,
    echConfigLabel: String,
    echDnsServerNameLabel: String,
    onEnabledChange: (Boolean) -> Unit,
    onServerNameChange: (String) -> Unit,
    onInsecureChange: (Boolean) -> Unit,
    onDisableSniChange: (Boolean) -> Unit,
    onAlpnChange: (String) -> Unit,
    onFingerprintChange: (String) -> Unit,
    onRealityEnabledChange: (Boolean) -> Unit,
    onRealityPublicKeyChange: (String) -> Unit,
    onRealityShortIdChange: (String) -> Unit,
    onEchEnabledChange: (Boolean) -> Unit,
    onEchConfigChange: (String) -> Unit,
    onEchQueryServerNameChange: (String) -> Unit,
    onImeDone: () -> Unit
) {
    Column(verticalArrangement = Arrangement.Top) {
        SmallTitle(
            text = stringResource(R.string.security_tls_config_security),
            insideMargin = PaddingValues(start = 26.dp, top = 8.dp, bottom = 8.dp, end = 26.dp)
        )
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 12.dp)
                .padding(bottom = 12.dp)
        ) {
            SwitchPreference(
                title = stringResource(R.string.enable_tls),
                checked = enabled,
                onCheckedChange = onEnabledChange
            )
        }
        if (enabled) {
            EditorField(
                serverName,
                stringResource(R.string.server_name),
                onServerNameChange,
                onImeDone
            )
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 12.dp)
            ) {
                SwitchPreference(
                    title = stringResource(R.string.allow_insecure),
                    checked = insecure,
                    onCheckedChange = onInsecureChange
                )
                SwitchPreference(
                    title = stringResource(R.string.disable_sni),
                    checked = disableSni,
                    onCheckedChange = onDisableSniChange
                )
            }
            EditorField(alpn, alpnLabel, onAlpnChange, onImeDone)
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 12.dp)
            ) {
                val fingerprints = listOf("chrome", "firefox", "safari", "randomised", "none")
                OverlayDropdownPreference(
                    title = utlsFingerprintLabel,
                    items = fingerprints,
                    selectedIndex = fingerprints.indexOf(fingerprint).coerceAtLeast(0),
                    onSelectedIndexChange = { index -> onFingerprintChange(fingerprints[index]) }
                )
                SwitchPreference(
                    title = stringResource(R.string.enable_reality),
                    checked = realityEnabled,
                    onCheckedChange = onRealityEnabledChange
                )
            }
            if (realityEnabled) {
                EditorField(
                    realityPublicKey,
                    realityPublicKeyLabel,
                    onRealityPublicKeyChange,
                    onImeDone
                )
                EditorField(realityShortId, realityShortIdLabel, onRealityShortIdChange, onImeDone)
            }
            Card(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 12.dp)
                    .padding(bottom = 12.dp)
            ) {
                SwitchPreference(
                    title = stringResource(R.string.enable_ech),
                    checked = echEnabled,
                    onCheckedChange = onEchEnabledChange
                )
            }
            if (echEnabled) {
                EditorField(echConfig, echConfigLabel, onEchConfigChange, onImeDone)
                EditorField(
                    echQueryServerName,
                    echDnsServerNameLabel,
                    onEchQueryServerNameChange,
                    onImeDone
                )
            }
        }
    }
}


