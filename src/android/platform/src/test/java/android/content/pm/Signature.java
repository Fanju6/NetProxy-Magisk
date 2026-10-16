package android.content.pm;

public final class Signature {
    private final byte[] bytes;
    public int reads;

    public Signature(byte[] bytes) {
        this.bytes = bytes.clone();
    }

    public byte[] toByteArray() {
        reads++;
        return bytes.clone();
    }
}
