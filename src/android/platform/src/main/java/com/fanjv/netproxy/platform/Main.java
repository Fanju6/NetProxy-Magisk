package com.fanjv.netproxy.platform;

import android.content.ContentProviderClient;
import android.content.ContentResolver;
import android.content.Context;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.ProviderInfo;
import android.content.pm.Signature;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Typeface;
import android.graphics.drawable.Drawable;
import android.os.Binder;
import android.os.IBinder;
import android.os.Build;
import android.os.Bundle;
import android.os.Looper;
import android.os.Process;
import android.os.UserHandle;
import android.os.UserManager;
import android.system.Os;
import org.json.JSONArray;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileOutputStream;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;

public final class Main {
    public static final String MANAGER_PACKAGE = "com.fanjv.netproxy";
    public static final String MANAGER_CERTIFICATE_SHA256 = "723e324ffc0564c20f51557fde05287d3b8695fa84a53da60b69154fa8458813";
    private static final String AUTHORITY = MANAGER_PACKAGE + ".connection";
    private static final int INPUT_LIMIT = 64 * 1024;
    private static final int ICON_SIZE = 128;

    public static void main(String[] args) {
        JSONObject response = new JSONObject();
        boolean success = false;
        try {
            response.put("schema", 1);
            response.put("ok", false);
            response.put("code", "android.query_failed");
            response.put("message", "Android 平台查询失败");
            if (Process.myUid() != 0) throw new SecurityException();
            JSONObject request = new JSONObject(readInput());
            String operation = request.getString("operation");
            Looper.prepareMainLooper();
            Class<?> threadClass = Class.forName("android.app.ActivityThread");
            Object thread = threadClass.getMethod("systemMain").invoke(null);
            Context context = (Context) threadClass.getMethod("getSystemContext").invoke(thread);
            Object result;
            if ("apps".equals(operation)) {
                if (request.has("user_id") || request.has("size")) throw new IllegalArgumentException();
                result = apps(context, new File(request.getString("icon_cache")));
            } else if ("provision".equals(operation)) {
                result = provision(context, request.getJSONObject("connection"));
            } else {
                throw new IllegalArgumentException();
            }
            response.put("ok", true);
            response.put("code", "android." + operation);
            response.put("message", "Android 平台查询完成");
            response.put("data", result);
            success = true;
        } catch (Throwable error) {
            // 系统异常可能包含输入资料，禁止将堆栈或异常文本写到输出。
        }
        System.out.println(response.toString());
        if (!success) System.exit(1);
    }

    private static String readInput() throws Exception {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        byte[] buffer = new byte[4096];
        int size;
        while ((size = System.in.read(buffer)) != -1) {
            if (output.size() + size > INPUT_LIMIT) throw new IllegalArgumentException();
            output.write(buffer, 0, size);
        }
        return new String(output.toByteArray(), StandardCharsets.UTF_8);
    }

    private static JSONObject apps(Context context, File cache) throws Exception {
        // 延迟字体的原始句柄可为 0，由框架 setDefault 解析并注册实际字体。
        Field fontMap = Typeface.class.getDeclaredField("sSystemFontMap");
        fontMap.setAccessible(true);
        Map<?, ?> fonts = (Map<?, ?>) fontMap.get(null);
        Typeface font = fonts == null ? null : (Typeface) fonts.get("sans-serif");
        if (font == null) {
            Typeface.class.getMethod("loadPreinstalledSystemFontMap").invoke(null);
            fonts = (Map<?, ?>) fontMap.get(null);
            font = fonts == null ? null : (Typeface) fonts.get("sans-serif");
        }
        if (font == null) throw new IllegalStateException();
        Method setDefault = Typeface.class.getDeclaredMethod("setDefault", Typeface.class);
        setDefault.setAccessible(true);
        setDefault.invoke(null, font);
        UserManager manager = context.getSystemService(UserManager.class);
        List<?> users = (List<?>) UserManager.class.getMethod("getUsers").invoke(manager);
        JSONArray result = new JSONArray();
        Set<String> retained = ConcurrentHashMap.newKeySet();
        ExecutorService workers = Executors.newFixedThreadPool(4);
        try {
            for (Object user : users) {
                Class<?> type = user.getClass();
                int id = type.getField("id").getInt(user);
                if (id < 0 || id > 21474) throw new IllegalArgumentException();
                Context userContext = (Context) Context.class.getMethod("createContextAsUser", UserHandle.class, int.class)
                    .invoke(context, UserHandle.getUserHandleForUid(id * 100000), 0);
                result.put(userApps(userContext, id, cache, retained, workers)
                    .put("id", id).put("name", (String) type.getField("name").get(user)));
            }
        } finally {
            workers.shutdownNow();
        }
        // 全部用户查询成功后才清理，共用目录不能按单个用户的清单删除。
        File[] files = cache.listFiles();
        if (files != null) for (File file : files) {
            String name = file.getName();
            if ((name.matches("[a-f0-9]{64}\\.png") && !retained.contains(name)) || name.matches("[a-f0-9]{64}\\.png\\..+\\.tmp")) file.delete();
        }
        return new JSONObject().put("users", result);
    }

    private static JSONObject userApps(Context context, int user, File cache, Set<String> retained,
        ExecutorService workers) throws Exception {
        PackageManager manager = context.getPackageManager();
        List<PackageInfo> applications = new ArrayList<>(manager.getInstalledPackages(0));
        Collections.sort(applications, new Comparator<PackageInfo>() {
            @Override public int compare(PackageInfo first, PackageInfo second) {
                return first.packageName.compareTo(second.packageName);
            }
        });
        String resources = context.getResources().getConfiguration().toString();
        JSONArray result = new JSONArray();
        List<Future<JSONObject>> entries = new ArrayList<>();
        for (PackageInfo installed : applications) {
            if (installed.applicationInfo != null) entries.add(workers.submit(() -> app(manager, installed, user, resources, cache, retained)));
        }
        for (Future<JSONObject> entry : entries) result.put(entry.get());
        String revision = digest((result.toString() + resources).getBytes(StandardCharsets.UTF_8));
        return new JSONObject().put("revision", revision).put("apps", result);
    }

    private static JSONObject app(PackageManager manager, PackageInfo installed, int user,
        String resources, File cache, Set<String> retained) throws Exception {
        ApplicationInfo info = installed.applicationInfo;
        long version = Build.VERSION.SDK_INT >= 28 ? installed.getLongVersionCode() : installed.versionCode;
        String label = manager.getApplicationLabel(info).toString();
        String iconKey = "";
        // 缺失图标由管理器显示占位图，系统默认图标仍可能触发厂商原生崩溃。
        if (cache != null && info.icon != 0) {
            String name = digest((user + ":" + info.packageName + ":" + version + ":" + installed.lastUpdateTime + ":" + resources)
                .getBytes(StandardCharsets.UTF_8)) + ".png";
            File target = new File(cache, name);
            try {
                if (!target.isFile() || target.length() == 0) writeIcon(manager, info, ICON_SIZE, target);
                retained.add(name);
                iconKey = name.substring(0, 64);
            } catch (Exception unavailable) {
                // 单个应用资源损坏或卸载不应阻止其他应用展示。
            }
        }
        return new JSONObject().put("package_name", info.packageName).put("uid", info.uid).put("label", label)
            .put("system", (info.flags & ApplicationInfo.FLAG_SYSTEM) != 0)
            .put("version_code", version).put("last_update_time", installed.lastUpdateTime).put("icon_key", iconKey);
    }

    private static void writeIcon(PackageManager manager, ApplicationInfo info, int size, File target) throws Exception {
        // loadIcon 的厂商主题扩展会访问仅注册应用进程可用的 Provider，直接读取原生图标资源。
        Drawable drawable = manager.getResourcesForApplication(info).getDrawable(info.icon, null);
        File temporary = File.createTempFile(target.getName() + ".", ".tmp", target.getParentFile());
        Bitmap image = null;
        try {
            image = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888);
            drawable.setBounds(0, 0, size, size);
            drawable.draw(new Canvas(image));
            try (FileOutputStream output = new FileOutputStream(temporary)) {
                Os.fchmod(output.getFD(), 0600);
                if (!image.compress(Bitmap.CompressFormat.PNG, 100, output)) throw new IllegalStateException();
                output.getFD().sync();
            }
            Os.rename(temporary.getPath(), target.getPath());
        } finally {
            temporary.delete();
            if (image != null) image.recycle();
        }
    }

    private static ProviderInfo verifyManager(PackageManager manager) throws Exception {
        int flags = Build.VERSION.SDK_INT >= 28 ? PackageManager.GET_SIGNING_CERTIFICATES : PackageManager.GET_SIGNATURES;
        PackageInfo installed = manager.getPackageInfo(MANAGER_PACKAGE, flags);
        if (installed.applicationInfo == null || installed.applicationInfo.uid / 100000 != 0) throw new SecurityException();
        String[] packages = manager.getPackagesForUid(installed.applicationInfo.uid);
        if (packages == null || packages.length != 1 || !MANAGER_PACKAGE.equals(packages[0])) throw new SecurityException();
        Signature[] signers = Build.VERSION.SDK_INT >= 28 ? installed.signingInfo.getApkContentsSigners() : installed.signatures;
        if (signers == null || signers.length != 1 || !MANAGER_CERTIFICATE_SHA256.equals(digest(signers[0].toByteArray()))) throw new SecurityException();
        ProviderInfo provider = manager.resolveContentProvider(AUTHORITY, 0);
        if (provider == null || !MANAGER_PACKAGE.equals(provider.packageName)
            || provider.applicationInfo.uid != installed.applicationInfo.uid || provider.exported) throw new SecurityException();
        return provider;
    }

    private static JSONObject provision(Context context, JSONObject connection) throws Exception {
        PackageManager manager = context.getPackageManager();
        verifyManager(manager);
        Class<?> activityType = Class.forName("android.app.IActivityManager");
        Object activity = Class.forName("android.app.ActivityManager").getMethod("getService").invoke(null);
        IBinder token = new Binder();
        // app_process 不在应用进程表中，必须使用系统外部 Provider 入口，并显式归还引用。
        Object holder = Build.VERSION.SDK_INT >= 29
            ? activityType.getMethod("getContentProviderExternal", String.class, int.class, IBinder.class, String.class)
                .invoke(activity, AUTHORITY, 0, token, "NetProxy")
            : activityType.getMethod("getContentProviderExternal", String.class, int.class, IBinder.class)
                .invoke(activity, AUTHORITY, 0, token);
        if (holder == null) throw new IllegalStateException();
        try {
            ProviderInfo expected = verifyManager(manager);
            ProviderInfo bound = (ProviderInfo) holder.getClass().getField("info").get(holder);
            if (!MANAGER_PACKAGE.equals(bound.packageName) || bound.exported
                || bound.applicationInfo.uid != expected.applicationInfo.uid) throw new SecurityException();
            Object remote = holder.getClass().getField("provider").get(holder);
            Class<?> providerType = Class.forName("android.content.IContentProvider");
            ContentProviderClient client = Build.VERSION.SDK_INT >= 29
                ? (ContentProviderClient) ContentProviderClient.class.getConstructor(ContentResolver.class, providerType, String.class, boolean.class)
                    .newInstance(context.getContentResolver(), remote, AUTHORITY, true)
                : (ContentProviderClient) ContentProviderClient.class.getDeclaredConstructor(ContentResolver.class, providerType, boolean.class)
                    .newInstance(context.getContentResolver(), remote, true);
            try (ContentProviderClient provider = client) {
                Bundle extras = new Bundle();
                extras.putString("connection", connection.toString());
                Bundle reply = provider.call("provision", null, extras);
                if (reply == null || !reply.getBoolean("accepted")) throw new IllegalStateException();
            }
        } finally {
            if (Build.VERSION.SDK_INT >= 29) {
                activityType.getMethod("removeContentProviderExternalAsUser", String.class, IBinder.class, int.class)
                    .invoke(activity, AUTHORITY, token, 0);
            } else {
                activityType.getMethod("removeContentProviderExternal", String.class, IBinder.class)
                    .invoke(activity, AUTHORITY, token);
            }
        }
        return new JSONObject().put("delivered", true);
    }

    private static String digest(byte[] bytes) throws Exception {
        byte[] hash = MessageDigest.getInstance("SHA-256").digest(bytes);
        StringBuilder text = new StringBuilder(64);
        for (byte value : hash) {
            text.append(Character.forDigit((value >>> 4) & 15, 16));
            text.append(Character.forDigit(value & 15, 16));
        }
        return text.toString();
    }
}
