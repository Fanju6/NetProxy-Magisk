package android.graphics;

import java.util.HashMap;
import java.util.Map;

public final class Typeface {
    public static Typeface DEFAULT;
    public static final Map<String, Typeface> sSystemFontMap = new HashMap<>();
    public final long native_instance;
    private final Typeface pendingTypeface;
    public static long registeredDefault;
    public static int loads;
    public static int registrations;
    public static boolean failLoad;
    public static boolean failRegistration;
    public static boolean keepPublicDefault;
    public static boolean deferredLoad;

    public Typeface(long pointer) {
        native_instance = pointer;
        pendingTypeface = null;
    }

    public Typeface(Typeface resolved) {
        native_instance = 0;
        pendingTypeface = resolved;
    }

    public long getNativeInstance() {
        if (pendingTypeface != null) return pendingTypeface.getNativeInstance();
        if (native_instance == 0) throw new IllegalStateException("The Typeface is not fully initialized.");
        return native_instance;
    }

    public static void loadPreinstalledSystemFontMap() {
        loads++;
        if (!failLoad) {
            Typeface font = new Typeface(1);
            if (deferredLoad) font = new Typeface(font);
            sSystemFontMap.put("sans-serif", font);
            if (!keepPublicDefault) DEFAULT = font;
            registeredDefault = 1;
        }
    }

    private static void setDefault(Typeface font) {
        if (failRegistration) throw new IllegalStateException();
        long pointer = font.getNativeInstance();
        registrations++;
        registeredDefault = pointer;
    }
}
