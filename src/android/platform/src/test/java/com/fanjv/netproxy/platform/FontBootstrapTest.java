package com.fanjv.netproxy.platform;

import android.content.Context;
import android.graphics.Typeface;
import java.io.File;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import org.junit.Before;
import org.junit.Test;
import static org.junit.Assert.*;

public final class FontBootstrapTest {
    private Method apps;

    @Before public void reset() throws Exception {
        Typeface.DEFAULT = null;
        Typeface.sSystemFontMap.clear();
        Typeface.registeredDefault = 0;
        Typeface.loads = 0;
        Typeface.registrations = 0;
        Typeface.failLoad = false;
        Typeface.failRegistration = false;
        Typeface.keepPublicDefault = false;
        Typeface.deferredLoad = false;
        apps = Main.class.getDeclaredMethod("apps", Context.class, File.class);
        apps.setAccessible(true);
    }

    @Test public void missingFontIsLoadedBeforeEnumeratingApplications() throws Exception {
        assertReady(1, 1);
    }

    @Test public void loadedDeferredFontRegistersItsResolvedNativeInstance() throws Exception {
        Typeface.deferredLoad = true;
        assertReady(1, 1);
        assertEquals(0, Typeface.sSystemFontMap.get("sans-serif").native_instance);
    }

    @Test public void existingDeferredFontDoesNotReloadOrReadItsRawHandle() throws Exception {
        Typeface font = new Typeface(new Typeface(new Typeface(7)));
        Typeface.DEFAULT = font;
        Typeface.sSystemFontMap.put("sans-serif", font);
        assertReady(0, 7);
        assertSame(font, Typeface.sSystemFontMap.get("sans-serif"));
        assertEquals(0, font.native_instance);
    }

    @Test public void loadedSystemFontDoesNotDependOnPublicDefaultBeingUpdated() throws Exception {
        Typeface.DEFAULT = new Typeface(0);
        Typeface.keepPublicDefault = true;
        assertReady(1, 1);
        assertEquals(0, Typeface.DEFAULT.native_instance);
    }

    @Test public void systemFontMapIsAuthoritativeRatherThanPublicDefault() throws Exception {
        Typeface.DEFAULT = new Typeface(0);
        Typeface.sSystemFontMap.put("sans-serif", new Typeface(7));
        assertReady(0, 7);
        assertEquals(0, Typeface.DEFAULT.native_instance);
    }

    @Test public void validPublicDefaultDoesNotHideMissingSystemFont() throws Exception {
        Typeface.DEFAULT = new Typeface(7);
        assertReady(1, 1);
    }

    @Test public void validHandleStillNeedsNativeDefaultRegistration() throws Exception {
        Typeface.DEFAULT = new Typeface(7);
        Typeface.sSystemFontMap.put("sans-serif", Typeface.DEFAULT);
        assertReady(0, 7);
    }

    @Test public void validFontIsPreservedAndRegistrationIsIdempotent() throws Exception {
        Typeface.DEFAULT = new Typeface(7);
        Typeface.sSystemFontMap.put("sans-serif", Typeface.DEFAULT);
        Typeface.registeredDefault = 7;
        assertReady(0, 7);
    }

    @Test public void failedLoadDoesNotRegisterNullFont() throws Exception {
        Typeface.failLoad = true;
        assertEquals(IllegalStateException.class, queryFailure().getClass());
        assertEquals(1, Typeface.loads);
        assertEquals(0, Typeface.registrations);
    }

    @Test public void frameworkRejectsUnresolvedFontBeforeApplicationQuery() throws Exception {
        Typeface.DEFAULT = new Typeface(0);
        Typeface.sSystemFontMap.put("sans-serif", Typeface.DEFAULT);
        assertEquals(IllegalStateException.class, queryFailure().getClass());
        assertEquals(0, Typeface.loads);
        assertEquals(0, Typeface.registrations);
    }

    @Test public void registrationFailureStopsBeforeApplicationQuery() throws Exception {
        Typeface.DEFAULT = new Typeface(7);
        Typeface.sSystemFontMap.put("sans-serif", Typeface.DEFAULT);
        Typeface.failRegistration = true;
        assertEquals(IllegalStateException.class, queryFailure().getClass());
        assertEquals(0, Typeface.registeredDefault);
    }

    private void assertReady(int loads, long pointer) throws Exception {
        // Null context marks entry into user enumeration after font initialization.
        assertEquals(NullPointerException.class, queryFailure().getClass());
        assertEquals(loads, Typeface.loads);
        assertEquals(pointer, Typeface.sSystemFontMap.get("sans-serif").getNativeInstance());
        assertEquals(pointer, Typeface.registeredDefault);
        assertEquals(1, Typeface.registrations);
    }

    private Throwable queryFailure() throws Exception {
        try {
            apps.invoke(null, null, new File("unused-icons"));
            throw new AssertionError("Expected query to stop");
        } catch (InvocationTargetException error) {
            Throwable cause = error.getCause();
            while (cause instanceof InvocationTargetException) cause = cause.getCause();
            return cause;
        }
    }
}
