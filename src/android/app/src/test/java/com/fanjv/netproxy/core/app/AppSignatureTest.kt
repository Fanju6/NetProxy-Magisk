package com.fanjv.netproxy.core.app

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppSignatureTest {
    @Test
    fun `sha256 fingerprint uses colon-separated uppercase hex`() {
        assertEquals(
            "BA:78:16:BF:8F:01:CF:EA:41:41:40:DE:5D:AE:22:23:"
                + "B0:03:61:A3:96:17:7A:9C:B4:10:FF:61:F2:00:15:AD",
            sha256Fingerprint("abc".toByteArray())
        )
    }

    @Test
    fun `matches Play app signing certificate but not upload certificate`() {
        assertTrue(
            matchesGooglePlayFingerprint(
                "24:54:65:F5:FF:D9:55:17:1A:AE:EA:FB:AC:AF:25:75:"
                    + "C0:42:07:8B:4E:F6:99:15:99:32:6A:53:D0:0F:C7:C1"
            )
        )
        assertFalse(
            matchesGooglePlayFingerprint(
                "72:3E:32:4F:FC:05:64:C2:0F:51:55:7F:DE:05:28:7D:"
                    + "3B:86:95:FA:84:A5:3D:A6:0B:69:15:4F:A8:45:88:13"
            )
        )
    }

    @Test
    fun `fingerprint matching ignores separators and letter case`() {
        assertTrue(
            matchesGooglePlayFingerprint(
                "245465f5ffd955171aaeeafb acaf2575c042078b4ef6991599326a53d00fc7c1"
            )
        )
    }
}
