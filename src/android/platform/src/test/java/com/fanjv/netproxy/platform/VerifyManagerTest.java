package com.fanjv.netproxy.platform;

import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.Signature;
import android.content.pm.SigningInfo;
import android.os.Build;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import org.junit.After;
import org.junit.Before;
import org.junit.Test;
import static org.junit.Assert.*;

public final class VerifyManagerTest {
    private PackageManager manager;
    private Signature untrusted;
    private Method verify;
    private int previousSdk;

    @Before public void prepare() throws Exception {
        previousSdk = Build.VERSION.SDK_INT;
        Build.VERSION.SDK_INT = 28;
        manager = new PackageManager();
        manager.installed = new PackageInfo();
        manager.installed.applicationInfo = new ApplicationInfo();
        manager.installed.applicationInfo.uid = 10123;
        manager.packages = new String[] {Main.MANAGER_PACKAGE};
        untrusted = new Signature("untrusted-fixture-certificate".getBytes(StandardCharsets.UTF_8));
        manager.installed.signatures = new Signature[] {untrusted};
        manager.installed.signingInfo = new SigningInfo();
        manager.installed.signingInfo.current = new Signature[] {untrusted};
        verify = Main.class.getDeclaredMethod("verifyManager", PackageManager.class);
        verify.setAccessible(true);
    }

    @After public void restore() {
        Build.VERSION.SDK_INT = previousSdk;
    }

    @Test public void missingApplicationIdentityIsRejectedBeforeSignerLookup() throws Exception {
        manager.installed.applicationInfo = null;
        assertRejected();
        assertEquals(0, manager.installed.signingInfo.reads);
    }

    @Test public void secondaryUserIsRejectedBeforeSignerLookup() throws Exception {
        manager.installed.applicationInfo.uid = 1010123;
        assertRejected();
        assertEquals(0, manager.installed.signingInfo.reads);
    }

    @Test public void uidMustBelongOnlyToTheOfficialPackage() throws Exception {
        for (String[] packages : new String[][] {null, {}, {"com.evil.app"}, {Main.MANAGER_PACKAGE, "com.evil.app"}}) {
            manager.packages = packages;
            assertRejected();
        }
        assertEquals(10123, manager.requestedUid);
        assertEquals(0, manager.installed.signingInfo.reads);
    }

    @Test public void modernSigningRequiresExactlyOneCurrentSigner() throws Exception {
        for (Signature[] signers : new Signature[][] {null, {}, {untrusted, untrusted}}) {
            manager.installed.signingInfo.current = signers;
            assertRejected();
        }
        assertEquals(3, manager.installed.signingInfo.reads);
        assertEquals(0, untrusted.reads);
    }

    @Test public void modernSigningRejectsWrongCertificateWithoutLegacyFallback() throws Exception {
        manager.installed.signatures = null;
        assertRejected();
        assertEquals(Main.MANAGER_PACKAGE, manager.requestedPackage);
        assertEquals(PackageManager.GET_SIGNING_CERTIFICATES, manager.requestedFlags);
        assertEquals(1, manager.installed.signingInfo.reads);
        assertEquals(1, untrusted.reads);
    }

    @Test public void legacySigningRequiresExactlyOneSigner() throws Exception {
        Build.VERSION.SDK_INT = 27;
        for (Signature[] signers : new Signature[][] {null, {}, {untrusted, untrusted}}) {
            manager.installed.signatures = signers;
            assertRejected();
        }
        assertEquals(PackageManager.GET_SIGNATURES, manager.requestedFlags);
        assertEquals(0, manager.installed.signingInfo.reads);
        assertEquals(0, untrusted.reads);
    }

    @Test public void legacySigningRejectsWrongCertificate() throws Exception {
        Build.VERSION.SDK_INT = 27;
        assertRejected();
        assertEquals(PackageManager.GET_SIGNATURES, manager.requestedFlags);
        assertEquals(0, manager.installed.signingInfo.reads);
        assertEquals(1, untrusted.reads);
    }

    private void assertRejected() throws Exception {
        try {
            verify.invoke(null, manager);
            fail("Expected untrusted manager to be rejected");
        } catch (InvocationTargetException error) {
            assertTrue("Expected SecurityException, not an incidental crash", error.getCause() instanceof SecurityException);
        }
        assertEquals(0, manager.providerLookups);
    }
}
