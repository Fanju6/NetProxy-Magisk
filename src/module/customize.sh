#!/system/bin/sh
#######################################
# 文件: customize.sh
# 功能: 选择安装模式、准备模块，在安装器结束后热切换模块目录。
# 用法: 由模块管理器加载；--apply-update 仅供本脚本后台提交使用。
# 依赖: 管理器 ui_print/grep_prop、BusyBox、Android su/getevent/pm。
#######################################

SKIPUNZIP=1
umask 077
readonly MODULE_ID=netproxy
readonly CONFIG_ENTRIES="config/module.conf config/ebpf/ebpf.conf config/singbox/config.json config/singbox/rules/local"
readonly EXECUTABLE_FILES="bin/sing-box bin/netproxyctl action.sh netproxyctl service.sh uninstall.sh"

INSTALL_MODE=fresh
LIVE_DIR=/data/adb/modules/netproxy
PROXY_WAS_RUNNING=false
SERVICE_STOPPED=false
BACKGROUND=false
KEY_PID=""

if [ "${1:-}" = --apply-update ]; then
  [ "$#" -eq 5 ] || exit 2
  BACKGROUND=true
  INSTALLER_PID="$2"
  MODPATH="$3"
  LIVE_DIR="$4"
  INSTALL_MODE="$5"
fi

# 参数: $1 标题。
# 返回: 0=已输出。
print_title() {
  ui_print ""
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━"
  ui_print "  $1"
  ui_print "━━━━━━━━━━━━━━━━━━━━━━━━━"
}

# 参数: $1 提示。
# 返回: 0=已输出。
print_step() { ui_print "▶ $1"; }
# 参数: $1 提示。
# 返回: 0=已输出。
print_ok() { ui_print "  ✓ $1"; }
# 参数: $1 提示。
# 返回: 0=已输出。
print_warn() { ui_print "  ⚠ $1"; }
# 参数: $1 提示。
# 返回: 0=已输出。
print_error() { ui_print "  ✗ $1"; }

# 参数: 无。
# 返回: 0=监听进程已回收。
stop_key_listener() {
  if [ -n "$KEY_PID" ]; then
    kill "$KEY_PID" 2>/dev/null || true
    wait "$KEY_PID" 2>/dev/null || true
    if [ "$KEY_FD" = 8 ]; then exec 8<&-; else exec 9<&-; fi
  fi
  KEY_PID=""
}

#######################################
# 参数: $1 等待秒数。
# 返回: 0=完成，1=事件文件失败；写入 VOLUME_KEY。
#######################################
wait_volume_key() {
  local remaining="$1" event count
  VOLUME_KEY=timeout
  if [ -z "$KEY_PID" ]; then
    : > "$INSTALL_TMP/keys" || return 1
    getevent -lq > "$INSTALL_TMP/keys" 2>/dev/null &
    KEY_PID=$!
    # Recovery 的 ui_print 使用 OUTFD，不能覆盖或关闭安装器的输出描述符。
    KEY_FD=9
    [ "${OUTFD:-}" != 9 ] || KEY_FD=8
    if [ "$KEY_FD" = 8 ]; then exec 8<"$INSTALL_TMP/keys"; else exec 9<"$INSTALL_TMP/keys"; fi
  fi
  while [ "$remaining" -gt 0 ]; do
    count=0
    while [ "$count" -lt 1000 ] && IFS= read -r event <&"$KEY_FD"; do
      count=$((count + 1))
      # getevent 会补齐尾部空格；只处理 DOWN，避免 UP 或长按重复改变选项。
      case "$event" in
        *EV_KEY*KEY_VOLUMEUP*DOWN*|*EV_KEY*KEY_VOLUMEUP*00000001*) VOLUME_KEY=up; return 0 ;;
        *EV_KEY*KEY_VOLUMEDOWN*DOWN*|*EV_KEY*KEY_VOLUMEDOWN*00000001*) VOLUME_KEY=down; return 0 ;;
      esac
    done
    sleep 1
    remaining=$((remaining - 1))
  done
}

# 参数: 无。
# 返回: 0=存在用户数据，1=首次安装。
has_existing_user_data() {
  [ -f "$LIVE_DIR/config/module.conf" ] || [ -d "$LIVE_DIR/data/catalog" ]
}

# 参数: 无。
# 返回: 0=已输出当前模式。
print_install_mode() {
  case "$INSTALL_MODE" in
    preserve) print_step "当前选择：保留现有数据" ;;
    nodes) print_step "当前选择：仅保留节点与订阅（配置恢复默认）" ;;
    fresh) print_step "当前选择：全新安装（不保留现有数据）" ;;
  esac
}

#######################################
# 参数: 无。
# 返回: 0=确认完成，1=取消或当前格式的数据不完整。
#######################################
choose_install_mode() {
  if ! has_existing_user_data; then
    INSTALL_MODE=fresh
    print_step "未发现现有用户数据，将执行全新安装"
    return 0
  fi
  INSTALL_MODE=preserve
  print_title "选择安装方式"
  ui_print ""
  ui_print "  1. 保留现有数据（默认）"
  ui_print "  2. 仅保留节点与订阅"
  ui_print "  3. 全新安装"
  ui_print ""
  ui_print "  [音量+] 循环选择  [音量-] 确认"
  ui_print "  未操作时，10 秒后保留现有数据"
  print_install_mode
  local timeout=10 interacted=false
  while :; do
    wait_volume_key "$timeout" || return 1
    case "$VOLUME_KEY" in
      up)
        case "$INSTALL_MODE" in
          preserve) INSTALL_MODE=nodes ;;
          nodes) INSTALL_MODE=fresh ;;
          fresh) INSTALL_MODE=preserve ;;
        esac
        interacted=true
        timeout=20
        print_install_mode
        ;;
      down)
        if [ "$INSTALL_MODE" != fresh ]; then break; fi
        stop_key_listener
        print_warn "全新安装将清除节点、订阅、配置和模块日志"
        ui_print "  [音量-] 再次确认  [音量+] 返回选择；10 秒无操作取消安装"
        wait_volume_key 10 || return 1
        case "$VOLUME_KEY" in
          down) break ;;
          up) INSTALL_MODE=preserve; interacted=true; timeout=20; print_install_mode ;;
          *) print_error "未确认全新安装，已取消"; return 1 ;;
        esac
        ;;
      *)
        if [ "$interacted" = true ]; then
          print_error "选择超时，已取消安装，现有数据未修改"
          return 1
        fi
        break
        ;;
    esac
  done
  stop_key_listener
  if [ "$INSTALL_MODE" != fresh ] && [ ! -d "$LIVE_DIR/data/catalog" ]; then
    print_error "当前 Catalog 不存在，无法保留节点与订阅"
    return 1
  fi
  if [ "$INSTALL_MODE" = preserve ]; then
    local entry
    for entry in $CONFIG_ENTRIES; do
      [ -e "$LIVE_DIR/$entry" ] || {
        print_error "当前用户配置不完整：$entry；请重新选择安装方式"
        return 1
      }
    done
  fi
  print_ok "安装方式已确认"
}

#######################################
# 参数: $1 目录，$2 目录权限，$3 文件权限。
# 返回: 0=成功，1=失败。
#######################################
set_directory_permissions() {
  [ -d "$1" ] || return 0
  find "$1" -type d -exec chmod "$2" {} + \
    && find "$1" -type f -exec chmod "$3" {} +
}

# 参数: 无。
# 返回: 0=成功，1=权限设置失败。
set_permissions() {
  chown -R 0:0 "$MODPATH" && chcon -R u:object_r:system_file:s0 "$MODPATH" || return 1
  local entry
  for entry in config data runtime logs; do
    set_directory_permissions "$MODPATH/$entry" 0700 0600 || return 1
  done
  # 不能先把用户配置设为公开可读，再收紧权限；中途失败会暴露凭据。
  find "$MODPATH" \( -path "$MODPATH/config" -o -path "$MODPATH/data" \
    -o -path "$MODPATH/runtime" -o -path "$MODPATH/logs" \) -prune \
    -o -type d -exec chmod 0755 {} + -o -type f -exec chmod 0644 {} + || return 1
  for entry in $EXECUTABLE_FILES; do
    [ ! -f "$MODPATH/$entry" ] || chmod 0755 "$MODPATH/$entry" || return 1
  done
}

# 参数: 无。
# 返回: 0=包结构有效，1=缺少当前模块所需文件。
validate_stage() {
  grep -qx "id=$MODULE_ID" "$MODPATH/module.prop" || return 1
  local entry
  for entry in bin/netproxyctl bin/sing-box netproxyctl service.sh \
    config/module.conf config/ebpf/ebpf.conf config/singbox/config.json \
    data/catalog/default/meta.json data/catalog/default/provider.json; do
    [ -s "$MODPATH/$entry" ] || return 1
  done
}

#######################################
# 参数: 无。
# 返回: 0=已停服或没有安装，1=停止失败。
#######################################
stop_proxy_if_running() {
  has_existing_user_data || return 0
  [ -x "$LIVE_DIR/bin/netproxyctl" ] || return 1
  if pidof -s "$LIVE_DIR/bin/sing-box" >/dev/null 2>&1; then
    PROXY_WAS_RUNNING=true
  fi
  # Worker 可能独立运行；即使核心已停止，也必须确认调度进程已退出。
  SERVICE_STOPPED=true
  "$LIVE_DIR/bin/netproxyctl" __internal worker stop --module-dir "$LIVE_DIR" >/dev/null 2>&1 \
    && "$LIVE_DIR/netproxyctl" service stop >/dev/null 2>&1
}

# 参数: 无。
# 返回: 0=成功或无需恢复，1=恢复失败。
restore_live_service() {
  [ "$SERVICE_STOPPED" = true ] || return 0
  local result=0
  su -c "\"$LIVE_DIR/bin/netproxyctl\" __internal worker start --module-dir \"$LIVE_DIR\"" >/dev/null 2>&1 || result=1
  if [ "$PROXY_WAS_RUNNING" = true ]; then
    su -c "\"$LIVE_DIR/netproxyctl\" service start" >/dev/null 2>&1 || result=1
  fi
  return "$result"
}

#######################################
# 参数: $1 目标 Catalog。
# 返回: 0=复制完成，1=分组不完整或复制失败。
#######################################
copy_catalog_state() {
  local target="$1" group file
  mkdir -p "$target" || return 1
  for group in "$LIVE_DIR/data/catalog"/*; do
    [ -d "$group" ] || continue
    [ "${group##*/}" != staging ] || continue
    [ -f "$group/meta.json" ] && [ -f "$group/provider.json" ] || return 1
    mkdir -p "$target/${group##*/}" || return 1
    for file in meta.json provider.json history.jsonl; do
      [ ! -f "$group/$file" ] || cp -p "$group/$file" "$target/${group##*/}/" || return 1
    done
  done
}

# 参数: 无。
# 返回: 0=输出保留清单，1=安装模式无效。
persistent_entries() {
  case "$INSTALL_MODE" in
    preserve) printf '%s\n' config data/catalog logs ;;
    nodes) printf '%s\n' data/catalog logs ;;
    fresh) ;;
    *) return 1 ;;
  esac
}

#######################################
# 参数: $1 快照目录。
# 返回: 0=回退成功，1=回退失败（保留快照）。
#######################################
rollback_snapshot() {
  local snapshot="$1" entry failed=0
  while IFS= read -r entry; do
    rm -rf "$MODPATH/$entry" || { failed=1; continue; }
    [ ! -e "$snapshot/previous/$entry" ] \
      || mv "$snapshot/previous/$entry" "$MODPATH/$entry" || failed=1
  done < "$snapshot/applied"
  return "$failed"
}

#######################################
# 参数: 无。
# 返回: 0=成功，1=复制或恢复失败；调用方必须持有数据锁。
#######################################
copy_user_data_locked() {
  [ "$INSTALL_MODE" != fresh ] || return 0
  local snapshot entries entry failed=false
  [ -d "$LIVE_DIR/data/catalog" ] || return 1
  [ ! -e "$LIVE_DIR/runtime/.config-apply" ] || return 1
  if [ "$INSTALL_MODE" = preserve ]; then
    for entry in $CONFIG_ENTRIES; do
      [ -e "$LIVE_DIR/$entry" ] || return 1
    done
  fi
  entries="$(persistent_entries)" || return 1
  snapshot="$(mktemp -d "$MODPATH/.install-state.XXXXXX")" || return 1
  : > "$snapshot/applied" || return 1
  for entry in $entries; do
    [ -e "$LIVE_DIR/$entry" ] || continue
    mkdir -p "$snapshot/current/$(dirname "$entry")" || { failed=true; break; }
    if [ "$entry" = data/catalog ]; then
      copy_catalog_state "$snapshot/current/$entry" || { failed=true; break; }
    else
      cp -a "$LIVE_DIR/$entry" "$snapshot/current/$entry" || { failed=true; break; }
      if [ "$entry" = config ]; then
        # 用户配置可包含核心持久状态；仅内置远程规则由本次安装包提供。
        rm -rf "$snapshot/current/config/singbox/rules/remote" \
          && cp -a "$MODPATH/config/singbox/rules/remote" "$snapshot/current/config/singbox/rules/remote" \
          || { failed=true; break; }
      fi
    fi
  done
  if [ "$failed" = false ]; then
    for entry in $entries; do
      [ -e "$snapshot/current/$entry" ] || continue
      mkdir -p "$snapshot/previous/$(dirname "$entry")" "$MODPATH/$(dirname "$entry")" \
        || { failed=true; break; }
      if [ -e "$MODPATH/$entry" ]; then
        mv "$MODPATH/$entry" "$snapshot/previous/$entry" || { failed=true; break; }
      fi
      if ! printf '%s\n' "$entry" >> "$snapshot/applied"; then
        [ ! -e "$snapshot/previous/$entry" ] || mv "$snapshot/previous/$entry" "$MODPATH/$entry" || return 1
        failed=true
        break
      fi
      mv "$snapshot/current/$entry" "$MODPATH/$entry" || { failed=true; break; }
    done
  fi
  if [ "$failed" = true ]; then
    rollback_snapshot "$snapshot" || return 1
    rm -rf "$snapshot"
    return 1
  fi
  rm -rf "$snapshot"
}

#######################################
# 参数: 锁文件列表、--、持锁回调。
# 返回: 0=成功，非零=锁忙或操作失败。
#######################################
with_catalog_group_locks() (
  [ "$1" != -- ] || { shift; "$@"; exit "$?"; }
  exec 4>"$1"
  flock -n 4 4>&4 || exit 1
  shift
  # 每层父 Shell 保留一个文件描述符，覆盖整个快照与目录切换。
  with_catalog_group_locks "$@"
)

# 参数: $@ 持锁回调。
# 返回: 0=成功，非零=根锁忙或操作失败。
with_catalog_root_lock() {
  flock -n 5 5>&5 || return 1
  "$@"
}

#######################################
# 参数: $@ 持锁期间执行的函数。
# 返回: 0=成功，非零=锁忙或操作失败。
#######################################
with_user_data_locks() (
  has_existing_user_data || { "$@"; exit "$?"; }
  # 与 Go 使用同一 inode 和固定顺序；不能通过删除锁文件解除互斥。
  mkdir -p /dev/netproxy || exit 1
  exec 9>/dev/netproxy/service.lock.flock
  # mksh 默认关闭外部命令的额外描述符，必须显式重定向传给 flock。
  flock -n 9 9>&9 || exit 1
  exec 8>"$LIVE_DIR/config/ebpf/ebpf.conf.lock"
  flock -n 8 8>&8 || exit 1
  exec 7>"$LIVE_DIR/config/module.conf.lock"
  flock -n 7 7>&7 || exit 1
  exec 6>"$LIVE_DIR/config/singbox/config.json.lock"
  flock -n 6 6>&6 || exit 1
  exec 3>/dev/netproxy/telemetry.lock.flock
  flock -n 3 3>&3 || exit 1
  local digest file
  digest="$(printf '%s\000root' "$LIVE_DIR/data/catalog" | sha256sum | cut -c1-16)"
  [ "${#digest}" -eq 16 ] || exit 1
  CATALOG_LOCK="data/.catalog.netproxy-$digest.lock"
  exec 5>"$LIVE_DIR/$CATALOG_LOCK"
  set -- -- with_catalog_root_lock "$@"
  for file in "$LIVE_DIR/data"/.catalog.netproxy-*.lock; do
    [ "$file" = "$LIVE_DIR/$CATALOG_LOCK" ] || set -- "$file" "$@"
  done
  # 分组锁先于根锁；任一锁忙立即中止，不与仍在运行的命令互相等待。
  with_catalog_group_locks "$@"
)

# 参数: 无。
# 返回: 0=当前事务已恢复且快照完成，1=失败。
synchronize_user_data() {
  [ "$INSTALL_MODE" = fresh ] || "$LIVE_DIR/netproxyctl" catalog list >/dev/null 2>&1 || return 1
  with_user_data_locks copy_user_data_locked
}

#######################################
# 参数: 透传 su 参数。
# 返回: exec 后由 root Shell 决定。
#######################################
launch_detached_root_shell() {
  if command -v setsid >/dev/null 2>&1; then exec setsid nohup su "$@"; fi
  exec nohup su "$@"
}

# 参数: $1 级别，$2 结果，$3 错误码，$4 固定消息。
# 返回: 0=完成。
write_log() {
  [ -d "$LIVE_DIR" ] || return 0
  mkdir -p "$LIVE_DIR/logs" 2>/dev/null || return 0
  printf '[%s] [%s] [module] [module.update] [%s] [%s] %s\n' \
    "$(date '+%Y-%m-%d %H:%M:%S')" "$1" "$2" "$3" "$4" >> "$LIVE_DIR/logs/service.log" 2>/dev/null || true
}

#######################################
# 参数: 无。
# 返回: 0=已切换，1=切换失败（保留当前模块与暂存目录）。
#######################################
commit_hot_update() {
  local backup_dir="$(dirname "$LIVE_DIR")/.netproxy.install-backup.$$" entry
  copy_user_data_locked && set_permissions || return 1
  rm -f "$MODPATH/data"/.catalog.netproxy-*.lock || return 1
  # 等待中的 Go 命令必须继续使用原来的锁，而不是目录切换后的第二个锁。
  if has_existing_user_data; then
    for entry in config/ebpf/ebpf.conf.lock config/module.conf.lock config/singbox/config.json.lock; do
      ln -f "$LIVE_DIR/$entry" "$MODPATH/$entry" || return 1
    done
    for entry in "$LIVE_DIR/data"/.catalog.netproxy-*.lock; do
      ln -f "$entry" "$MODPATH/data/${entry##*/}" || return 1
    done
  fi
  [ ! -e "$backup_dir" ] || return 1
  rm -f "$MODPATH/update" || return 1
  mv "$LIVE_DIR" "$backup_dir" || return 1
  if ! mv "$MODPATH" "$LIVE_DIR"; then
    mv "$backup_dir" "$LIVE_DIR" || write_log ERROR failed module.restore_failed "模块目录恢复失败，请勿重启并检查安装目录"
    return 1
  fi
  rm -rf "$backup_dir" || write_log WARN failed module.backup_cleanup_failed "安装备份未能删除，新版本已应用"
  return 0
}

#######################################
# 参数: 无。
# 返回: 0=完成，1=热切换未完成，保留管理器更新路径。
#######################################
apply_hot_update() {
  local elapsed=0
  case "$INSTALL_MODE" in preserve|nodes|fresh) ;; *) return 1 ;; esac
  while [ -d "/proc/$INSTALLER_PID" ] || [ ! -f "$LIVE_DIR/update" ]; do
    [ "$elapsed" -lt 60 ] || return 1
    sleep 1
    elapsed=$((elapsed + 1))
  done
  # 管理器在 customize.sh 返回后仍会复制 module.prop 并写入 update。
  sleep 3
  validate_stage && [ -f "$LIVE_DIR/update" ] || return 1
  stop_proxy_if_running || return 1
  [ "$INSTALL_MODE" = fresh ] || "$LIVE_DIR/netproxyctl" catalog list >/dev/null 2>&1 || return 1
  with_user_data_locks commit_hot_update || return 1
  write_log INFO success - "后台热更新已完成，无需重启设备"
  SERVICE_STOPPED=true
  restore_live_service || write_log WARN failed service.restore_failed "新版已应用，后台 Worker 或服务恢复失败，请在管理器中检查"
}

# 参数: 无。
# 返回: 0=已安排，1=无法启动后台 Shell。
schedule_hot_update() {
  command -v su >/dev/null 2>&1 || return 1
  # stdin 持有脚本内容，安装器删除 customize.sh 或临时目录不影响后台提交。
  (launch_detached_root_shell -c "/system/bin/sh -s -- --apply-update '$$' '$MODPATH' '$LIVE_DIR' '$INSTALL_MODE'" \
    < "$MODPATH/customize.sh") >/dev/null 2>&1 &
}

# 参数: 无。
# 返回: 0=完成（APK 安装失败不阻塞），1=包损坏、按键读取或 APK 清理失败。
install_bundled_manager() {
  local install_output
  print_title "安装 NetProxy 管理器"
  ui_print ""
  if [ ! -s "$MODPATH/NetProxy.apk" ]; then
    print_error "安装包缺少 NetProxy 管理器 APK"
    return 1
  fi
  ui_print "  [音量+] 安装或更新（默认）"
  ui_print "  [音量-] 跳过"
  ui_print "  10 秒未操作，默认安装"
  stop_key_listener
  wait_volume_key 10 || return 1
  stop_key_listener
  if [ "$VOLUME_KEY" = down ]; then
    print_step "已跳过管理器安装"
  else
    # PackageManager 经 Binder 接收输出描述符，不能直接写入安装器的 system_file 日志。
    if install_output="$(pm install -r "$MODPATH/NetProxy.apk" < /dev/null 2>&1)"; then
      print_ok "管理器安装成功"
    else
      print_warn "管理器安装失败，模块安装继续；未卸载或清除现有应用"
      ui_print "  $install_output"
    fi
  fi
  rm -f "$MODPATH/NetProxy.apk"
}

# 参数: 无。
# 返回: 安装退出码；回收按键监听和临时文件。
finish_install() {
  local result="$?"
  trap - EXIT HUP INT TERM
  stop_key_listener
  [ ! -d "${INSTALL_TMP:-}" ] || rm -rf "$INSTALL_TMP"
  return "$result"
}

if [ "$BACKGROUND" = true ]; then
  if ! apply_hot_update; then
    write_log WARN failed module.update_failed "后台热更新未提交，保留暂存模块，由管理器下次开机处理"
    restore_live_service || write_log WARN failed service.restore_failed "安装前的 Worker 或服务恢复失败"
    exit 1
  fi
  exit 0
fi

INSTALL_TMP="$(mktemp -d "$TMPDIR/netproxy-install.XXXXXX")" || exit 1
trap finish_install EXIT
trap 'exit 1' HUP INT TERM

# Recovery 可以直接写入空的最终目录；升级仍必须使用独立暂存目录。
[ -d "$MODPATH" ] || exit 1
if [ "$(cd "$MODPATH" && pwd -P)" = "$LIVE_DIR" ]; then
  [ "${BOOTMODE:-false}" != true ] && ! has_existing_user_data || exit 1
fi
unzip -o "$ZIPFILE" module.prop -d "$INSTALL_TMP" >/dev/null 2>&1 || exit 1
grep -qx "id=$MODULE_ID" "$INSTALL_TMP/module.prop" || exit 1
print_title "NetProxy - sing-box 透明代理"
ui_print ""
ui_print "  版本: $(grep_prop version "$INSTALL_TMP/module.prop")"
choose_install_mode || exit 1
print_title "准备安装"
print_step "解压与校验安装包..."
unzip -o "$ZIPFILE" -x 'META-INF/*' -d "$MODPATH" >/dev/null 2>&1 \
  && [ -s "$MODPATH/NetProxy.apk" ] && validate_stage && set_permissions \
  || { print_error "安装包或权限检查失败"; exit 1; }
install_bundled_manager || { print_error "管理器安装步骤失败"; exit 1; }
print_title "安装模块"
ui_print "  安装完成前请勿修改模块配置、节点或订阅"
synchronize_user_data && set_permissions || { print_error "保留用户数据或权限设置失败"; exit 1; }
if [ "${BOOTMODE:-false}" = true ]; then
  NETPROXY_MODULE_DIR="$MODPATH" "$MODPATH/bin/netproxyctl" config check > "$INSTALL_TMP/check.log" 2>&1 \
    || { print_error "配置检查失败，未替换当前模块；请检查安装模式与当前配置"; exit 1; }
  print_ok "模块配置检查通过"
fi

if [ "${BOOTMODE:-false}" = true ] && schedule_hot_update; then
  print_title "安装完成"
  ui_print "  正在后台应用新版本，无需重启设备"
  ui_print "  接下来约 3 秒请不要重启；若立即重启，"
  ui_print "  模块管理器将按标准流程应用暂存模块"
else
  print_title "安装完成"
  ui_print "  请重启设备应用新版本"
fi
finish_install
