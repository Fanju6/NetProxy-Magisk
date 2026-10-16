package android.content.pm;

// Host-only boundary double, following the platform's Typeface test pattern.
public final class PackageManager {
    public static final int GET_SIGNATURES = 64;
    public static final int GET_SIGNING_CERTIFICATES = 134217728;
    public PackageInfo installed;
    public String[] packages;
    public String requestedPackage;
    public int requestedFlags;
    public int requestedUid;
    public int providerLookups;

    public PackageInfo getPackageInfo(String name, int flags) {
        requestedPackage = name;
        requestedFlags = flags;
        return installed;
    }

    public String[] getPackagesForUid(int uid) {
        requestedUid = uid;
        return packages;
    }

    public ProviderInfo resolveContentProvider(String authority, int flags) {
        providerLookups++;
        throw new AssertionError("Untrusted package reached provider resolution");
    }
}
