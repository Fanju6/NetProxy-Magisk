package android.content.pm;

public final class SigningInfo {
    public Signature[] current;
    public int reads;

    public Signature[] getApkContentsSigners() {
        reads++;
        return current;
    }
}
