package com.fanjv.netproxy;

import android.app.Activity;
import android.app.Application;
import android.app.Instrumentation;
import android.content.Intent;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.View;
import android.view.ViewGroup;
import android.webkit.WebView;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicReference;

/** 只进入测试 APK，不向生产壳添加调试命令或 Root 能力。 */
public final class LifecycleProbe extends Instrumentation {
    private final AtomicReference<Activity> current = new AtomicReference<>();
    private final AtomicInteger destroyed = new AtomicInteger();

    @Override public void onCreate(Bundle args) {
        super.onCreate(args);
        start();
    }

    @Override public void onStart() {
        Bundle result = new Bundle();
        Application app = null;
        Application.ActivityLifecycleCallbacks callbacks = new Application.ActivityLifecycleCallbacks() {
            @Override public void onActivityCreated(Activity a, Bundle b) { current.set(a); }
            @Override public void onActivityDestroyed(Activity a) { destroyed.incrementAndGet(); }
            @Override public void onActivityStarted(Activity a) {}
            @Override public void onActivityResumed(Activity a) {}
            @Override public void onActivityPaused(Activity a) {}
            @Override public void onActivityStopped(Activity a) {}
            @Override public void onActivitySaveInstanceState(Activity a, Bundle b) {}
        };
        try {
            Activity first = startActivitySync(new Intent(Intent.ACTION_MAIN)
                .setClassName(getTargetContext(), "com.fanjv.netproxy.MainActivity")
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK));
            current.set(first);
            app = first.getApplication();
            app.registerActivityLifecycleCallbacks(callbacks);
            verifyRead(first);
            for (int i = 0; i < 2; i++) {
                Activity old = current.get();
                String begin = "Object.defineProperty(document,'visibilityState',{configurable:true,get:()=> 'hidden'});"
                    + "document.dispatchEvent(new Event('visibilitychange'));"
                    + "window.__probe=false;window.__TAURI_INTERNALS__.invoke('plugin:module-api|open_events',"
                    + "{subscriptionId:'activity-probe-" + i + "',topics:['status','groups','subscriptions']})"
                    + ".then(()=>window.__probe=true).catch(()=>window.__probe='error');";
                evaluate(old, begin);
                waitValue(old, "window.__probe", "true");
                runOnMainSync(old::recreate);
                long deadline = SystemClock.uptimeMillis() + 15_000;
                while ((current.get() == old || !old.isDestroyed()) && SystemClock.uptimeMillis() < deadline) {
                    SystemClock.sleep(50);
                }
                if (current.get() == old || !old.isDestroyed()) throw new AssertionError("Activity not recreated");
                verifyRead(current.get());
            }
            result.putInt("recreated", destroyed.get());
            result.putBoolean("read_after_recreate", true);
            result.putBoolean("sse_open_before_destroy", true);
            finish(Activity.RESULT_OK, result);
        } catch (Throwable failure) {
            result.putString("failure", failure.getClass().getSimpleName());
            // 不将包含设备内容的异常原文写入 instrumentation 输出。
            finish(Activity.RESULT_CANCELED, result);
        } finally {
            if (app != null) app.unregisterActivityLifecycleCallbacks(callbacks);
        }
    }

    private void verifyRead(Activity activity) throws Exception {
        long deadline = SystemClock.uptimeMillis() + 20_000;
        while (SystemClock.uptimeMillis() < deadline) {
            if ("true".equals(evaluate(activity, "Boolean(window.__TAURI_INTERNALS__?.invoke)"))) break;
            SystemClock.sleep(100);
        }
        evaluate(activity, "window.__probeRead=null;window.__TAURI_INTERNALS__.invoke('plugin:module-api|read',"
            + "{requestId:crypto.randomUUID(),resource:'info'}).then(r=>window.__probeRead=r.status)"
            + ".catch(e=>window.__probeRead=e.code);");
        waitValue(activity, "window.__probeRead", "200");
    }

    private void waitValue(Activity activity, String expression, String expected) throws Exception {
        long deadline = SystemClock.uptimeMillis() + 20_000;
        while (SystemClock.uptimeMillis() < deadline) {
            if (expected.equals(evaluate(activity, expression))) return;
            SystemClock.sleep(100);
        }
        throw new AssertionError("Native lifecycle probe timed out");
    }

    private String evaluate(Activity activity, String script) throws Exception {
        AtomicReference<String> response = new AtomicReference<>();
        runOnMainSync(() -> {
            WebView view = webView(activity.getWindow().getDecorView());
            if (view != null) view.evaluateJavascript(script, response::set);
            else response.set("null");
        });
        long deadline = SystemClock.uptimeMillis() + 5_000;
        while (response.get() == null && SystemClock.uptimeMillis() < deadline) SystemClock.sleep(20);
        return response.get();
    }

    private WebView webView(View view) {
        if (view instanceof WebView) return (WebView) view;
        if (view instanceof ViewGroup) {
            ViewGroup group = (ViewGroup) view;
            for (int i = 0; i < group.getChildCount(); i++) {
                WebView found = webView(group.getChildAt(i));
                if (found != null) return found;
            }
        }
        return null;
    }
}
