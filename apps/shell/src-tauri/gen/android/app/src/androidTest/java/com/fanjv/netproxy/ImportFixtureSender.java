package com.fanjv.netproxy;

import android.app.Activity;
import android.content.ClipData;
import android.content.Intent;
import android.net.Uri;
import android.os.Bundle;

/** Sends real Android read grants for anonymous fixtures; not present in production. */
public final class ImportFixtureSender extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        String name = getIntent().getStringExtra("fixture");
        if (name == null || !java.util.Arrays.asList("nodes.txt", "config.json", "invalid.json", "binary.json", "large.json").contains(name)) {
            finish(); return;
        }
        Uri uri = Uri.parse("content://" + getPackageName() + ".m5-fixtures/" + name);
        Intent share = new Intent(Intent.ACTION_SEND).setType("application/json")
            .setClassName(getPackageName().replaceFirst("\\.test$", ""), "com.fanjv.netproxy.MainActivity")
            .putExtra(Intent.EXTRA_STREAM, uri)
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        share.setClipData(ClipData.newRawUri("M5 fixture", uri));
        startActivity(share);
        finish();
    }
}
