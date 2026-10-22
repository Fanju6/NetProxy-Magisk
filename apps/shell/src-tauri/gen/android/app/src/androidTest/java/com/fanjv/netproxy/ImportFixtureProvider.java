package com.fanjv.netproxy;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.OpenableColumns;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;

/** Anonymous fixtures only, compiled solely into the instrumentation APK. */
public final class ImportFixtureProvider extends ContentProvider {
    @Override public boolean onCreate() { return true; }
    private byte[] content(Uri uri) throws FileNotFoundException {
        switch (String.valueOf(uri.getLastPathSegment())) {
            case "nodes.txt": return "trojan://fixture@example.invalid:443#M5-One\ntrojan://fixture@example.invalid:444#M5-Two".getBytes(StandardCharsets.UTF_8);
            case "config.json": return "{\"outbounds\":[{\"type\":\"direct\",\"tag\":\"M5-Direct\"}],\"route\":{\"final\":\"M5-Direct\"}}".getBytes(StandardCharsets.UTF_8);
            case "invalid.json": return "{broken JSON".getBytes(StandardCharsets.UTF_8);
            case "binary.json": return new byte[] {(byte) 0xc3, 0x28};
            case "large.json": return new byte[1024 * 1024 + 1];
            default: throw new FileNotFoundException("Unknown fixture");
        }
    }
    @Override public String getType(Uri uri) { return "application/json"; }
    @Override public Cursor query(Uri uri, String[] projection, String selection, String[] args, String sort) {
        MatrixCursor cursor = new MatrixCursor(new String[] {OpenableColumns.DISPLAY_NAME, OpenableColumns.SIZE});
        try { cursor.addRow(new Object[] {uri.getLastPathSegment(), content(uri).length}); }
        catch (FileNotFoundException ignored) { }
        return cursor;
    }
    @Override public ParcelFileDescriptor openFile(Uri uri, String mode) throws FileNotFoundException {
        if (!"r".equals(mode)) throw new FileNotFoundException("Read only");
        byte[] bytes = content(uri);
        File file = new File(getContext().getCacheDir(), uri.getLastPathSegment());
        try (FileOutputStream stream = new FileOutputStream(file)) { stream.write(bytes); }
        catch (Exception ignored) { throw new FileNotFoundException("Fixture unavailable"); }
        return ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY);
    }
    @Override public Uri insert(Uri uri, ContentValues values) { throw new UnsupportedOperationException(); }
    @Override public int delete(Uri uri, String selection, String[] args) { throw new UnsupportedOperationException(); }
    @Override public int update(Uri uri, ContentValues values, String selection, String[] args) { throw new UnsupportedOperationException(); }
}
