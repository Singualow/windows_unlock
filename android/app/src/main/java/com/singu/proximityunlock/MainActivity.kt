package com.singu.proximityunlock

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.app.KeyguardManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.content.res.ColorStateList
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.graphics.drawable.RippleDrawable
import android.os.Bundle
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.WindowInsets
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class MainActivity : Activity() {
    private lateinit var store: SecureStore
    private lateinit var scrollView: ScrollView
    private lateinit var heroCard: LinearLayout
    private lateinit var statusTitle: TextView
    private lateinit var statusSubtitle: TextView
    private lateinit var serviceChip: TextView
    private lateinit var modeChip: TextView
    private lateinit var pairingChip: TextView
    private lateinit var serviceButton: TextView
    private lateinit var strictModeCard: LinearLayout
    private lateinit var convenienceModeCard: LinearLayout
    private lateinit var strictModeCheck: TextView
    private lateinit var convenienceModeCheck: TextView
    private lateinit var deviceCard: LinearLayout
    private lateinit var deviceTitle: TextView
    private lateinit var deviceBadge: TextView
    private lateinit var deviceSubtitle: TextView
    private lateinit var keyBackend: TextView
    private lateinit var pairingPanel: LinearLayout
    private lateinit var uriInput: EditText
    private lateinit var revokeButton: TextView
    private lateinit var logContainer: LinearLayout
    private var statusReceiverRegistered = false

    private val statusReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) = renderStatus()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        store = SecureStore(this)
        buildUi()
        intent?.dataString?.let(::showPairingUri)
        requestPermissionsIfNeeded()
        renderStatus()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        intent.dataString?.let(::showPairingUri)
    }

    override fun onStart() {
        super.onStart()
        registerReceiver(
            statusReceiver,
            IntentFilter(UnlockService.ACTION_STATUS_CHANGED),
            RECEIVER_NOT_EXPORTED,
        )
        statusReceiverRegistered = true
        if (store.enabled() && hasBluetoothPermissions()) UnlockService.start(this)
        renderStatus()
    }

    override fun onStop() {
        if (statusReceiverRegistered) {
            unregisterReceiver(statusReceiver)
            statusReceiverRegistered = false
        }
        super.onStop()
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == PERMISSION_REQUEST && hasBluetoothPermissions() && store.enabled()) {
            UnlockService.start(this)
        }
        renderStatus()
    }

    private fun buildUi() {
        scrollView = ScrollView(this).apply {
            isFillViewport = true
            setBackgroundColor(COLOR_PAGE)
            clipToPadding = false
        }
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(18), dp(18), dp(18), dp(38))
        }
        scrollView.addView(content, ViewGroup.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        scrollView.setOnApplyWindowInsetsListener { _, insets ->
            val bars = insets.getInsets(WindowInsets.Type.systemBars())
            content.setPadding(dp(18), dp(18) + bars.top, dp(18), dp(38) + bars.bottom)
            insets
        }

        content.addView(buildHeader())
        content.addView(buildHero(), verticalParams(top = 18))
        content.addView(buildModeCard(), verticalParams(top = 14))
        content.addView(buildDeviceCard(), verticalParams(top = 14))
        content.addView(buildLogsCard(), verticalParams(top = 14))
        content.addView(text("▣  不记录密码、私钥、完整签名或设备标识", 12f, COLOR_MUTED).apply {
            gravity = Gravity.CENTER
            setPadding(0, dp(20), 0, 0)
        })

        setContentView(scrollView)
    }

    private fun buildHeader(): View {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
        }
        row.addView(text("ᛒ", 27f, Color.WHITE, true).apply {
            gravity = Gravity.CENTER
            background = gradient(intArrayOf(COLOR_BLUE, 0xFF70A0FF.toInt()), 15)
            elevation = dp(3).toFloat()
        }, LinearLayout.LayoutParams(dp(48), dp(48)))
        val copy = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(12), 0, 0, 0)
        }
        copy.addView(text("蓝牙解锁", 22f, COLOR_TEXT, true))
        copy.addView(text("Android 安全钥匙", 12f, COLOR_MUTED).apply { setPadding(0, dp(2), 0, 0) })
        row.addView(copy, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        val version = packageManager.getPackageInfo(packageName, 0).versionName ?: ""
        row.addView(text("v$version", 12f, COLOR_BLUE, true).apply {
            gravity = Gravity.CENTER
            setPadding(dp(11), dp(7), dp(11), dp(7))
            background = shape(COLOR_BLUE_SOFT, 14)
        })
        return row
    }

    private fun buildHero(): View {
        heroCard = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(22), dp(22), dp(22), dp(20))
            background = gradient(intArrayOf(0xFFF0F5FF.toInt(), 0xFFECFAF6.toInt()), 24)
            elevation = dp(2).toFloat()
        }
        heroCard.addView(text("当前状态", 12f, COLOR_BLUE, true).apply { letterSpacing = .08f })
        statusTitle = text("正在读取安全状态", 24f, COLOR_TEXT, true).apply { setPadding(0, dp(8), 0, 0) }
        statusSubtitle = text("请稍候…", 14f, COLOR_SUBTEXT).apply { setPadding(0, dp(6), 0, 0) }
        heroCard.addView(statusTitle)
        heroCard.addView(statusSubtitle)

        val chips = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            setPadding(0, dp(17), 0, 0)
        }
        serviceChip = chip("服务检查中")
        modeChip = chip("安全模式")
        pairingChip = chip("等待配对")
        listOf(serviceChip, modeChip, pairingChip).forEachIndexed { index, chip ->
            chips.addView(chip, LinearLayout.LayoutParams(0, dp(34), 1f).apply { if (index > 0) marginStart = dp(7) })
        }
        heroCard.addView(chips)

        val actions = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            setPadding(0, dp(18), 0, 0)
        }
        serviceButton = actionButton("启动服务", true) { toggleService() }
        val pairButton = actionButton("管理配对", false) { togglePairingPanel(true) }
        actions.addView(serviceButton, LinearLayout.LayoutParams(0, dp(48), 1.25f))
        actions.addView(pairButton, LinearLayout.LayoutParams(0, dp(48), 1f).apply { marginStart = dp(10) })
        heroCard.addView(actions)
        return heroCard
    }

    private fun buildModeCard(): View {
        val card = sectionCard()
        card.addView(sectionTitle("解锁模式", "请与电脑端保持一致，仅手机解锁后可切换"))
        val strict = modeRow(
            badge = "盾",
            title = "安全模式",
            subtitle = "手机解锁后才响应电脑认证",
            onClick = { switchMode(false) },
        )
        strictModeCard = strict.first
        strictModeCheck = strict.second
        card.addView(strictModeCard, verticalParams(top = 15, height = 74))
        val convenience = modeRow(
            badge = "便",
            title = "便捷模式",
            subtitle = "手机锁屏时仍可响应认证",
            onClick = { switchMode(true) },
        )
        convenienceModeCard = convenience.first
        convenienceModeCheck = convenience.second
        card.addView(convenienceModeCard, verticalParams(top = 8, height = 74))
        return card
    }

    private fun buildDeviceCard(): View {
        deviceCard = sectionCard()
        val heading = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
        }
        val copy = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        deviceTitle = text("已配对的电脑", 18f, COLOR_TEXT, true)
        deviceSubtitle = text("Windows 11 · 加密挑战响应", 13f, COLOR_MUTED).apply { setPadding(0, dp(4), 0, 0) }
        copy.addView(deviceTitle)
        copy.addView(deviceSubtitle)
        heading.addView(copy, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        deviceBadge = text("已配对", 12f, COLOR_GREEN, true).apply {
            gravity = Gravity.CENTER
            setPadding(dp(11), dp(7), dp(11), dp(7))
            background = shape(COLOR_GREEN_SOFT, 14)
        }
        heading.addView(deviceBadge)
        deviceCard.addView(heading)
        keyBackend = text("正在读取硬件密钥…", 13f, COLOR_SUBTEXT).apply {
            setPadding(dp(13), dp(11), dp(13), dp(11))
            background = shape(0xFFF7F9FD.toInt(), 12)
        }
        deviceCard.addView(keyBackend, verticalParams(top = 15))

        val deviceActions = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
        }
        deviceActions.addView(actionButton("添加或重新配对", false) { togglePairingPanel() }, LinearLayout.LayoutParams(0, dp(44), 1f))
        revokeButton = actionButton("撤销设备", false) { confirmRevoke() }.apply {
            setTextColor(COLOR_RED)
            background = ripple(0xFFFFF7F7.toInt(), 12, 0xFFFFD9D9.toInt())
        }
        deviceActions.addView(revokeButton, LinearLayout.LayoutParams(0, dp(44), .72f).apply { marginStart = dp(9) })
        deviceCard.addView(deviceActions, verticalParams(top = 12))

        pairingPanel = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            visibility = View.GONE
            setPadding(dp(14), dp(14), dp(14), dp(14))
            background = shape(0xFFF7FAFF.toInt(), 15, 0xFFDCE7FA.toInt())
        }
        pairingPanel.addView(text("电脑配对信息", 14f, COLOR_TEXT, true))
        pairingPanel.addView(text("扫描电脑二维码后会自动填入，也可以粘贴配对链接。", 12f, COLOR_MUTED).apply {
            setPadding(0, dp(4), 0, dp(10))
        })
        uriInput = EditText(this).apply {
            hint = "proximityunlock://pair…"
            textSize = 13f
            setTextColor(COLOR_TEXT)
            setHintTextColor(0xFF9AA6B8.toInt())
            minLines = 2
            maxLines = 4
            setPadding(dp(12), dp(10), dp(12), dp(10))
            background = shape(Color.WHITE, 11, 0xFFD7E1F0.toInt())
        }
        pairingPanel.addView(uriInput, verticalParams(height = 76))
        pairingPanel.addView(actionButton("保存配对并启动", true) { savePairing() }, verticalParams(top = 10, height = 45))
        deviceCard.addView(pairingPanel, verticalParams(top = 12))
        return deviceCard
    }

    private fun buildLogsCard(): View {
        val card = sectionCard()
        val heading = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
        }
        heading.addView(sectionTitle("安全日志", "仅保留本机脱敏事件"), LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        heading.addView(text("清空", 13f, COLOR_BLUE, true).apply {
            gravity = Gravity.CENTER
            setPadding(dp(10), dp(8), dp(10), dp(8))
            background = ripple(COLOR_BLUE_SOFT, 12)
            setOnClickListener {
                store.clearEvents()
                renderLogs()
            }
        })
        card.addView(heading)
        logContainer = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(0, dp(12), 0, 0)
        }
        card.addView(logContainer)
        return card
    }

    private fun renderStatus() {
        val paired = store.isPaired()
        val enabled = store.enabled()
        val runtime = store.runtimeStatus()
        val convenience = store.convenienceAllowed()
        val ready = paired && enabled && runtime.contains("广播运行中")

        statusTitle.text = when {
            ready -> "手机已连接，解锁准备就绪"
            !paired -> "等待连接你的电脑"
            enabled -> "蓝牙钥匙正在准备"
            else -> "蓝牙钥匙已停止"
        }
        statusSubtitle.text = when {
            !paired -> "请从电脑端生成二维码完成首次配对"
            else -> "$runtime · ${if (convenience) "便捷模式" else "安全模式"}"
        }
        heroCard.background = if (ready) {
            gradient(intArrayOf(0xFFF0F6FF.toInt(), 0xFFE9FAF3.toInt()), 24)
        } else {
            gradient(intArrayOf(0xFFF2F6FF.toInt(), 0xFFF6F8FC.toInt()), 24)
        }
        styleChip(serviceChip, if (ready) "✓ 服务正常" else if (enabled) "● 服务准备中" else "○ 服务已停止", ready)
        modeChip.text = if (convenience) "便捷模式" else "安全模式"
        modeChip.setTextColor(COLOR_BLUE)
        modeChip.background = shape(COLOR_BLUE_SOFT, 17)
        styleChip(pairingChip, if (paired) "✓ 已配对" else "○ 未配对", paired)
        serviceButton.text = if (enabled) "停止蓝牙钥匙" else "启动蓝牙钥匙"
        serviceButton.background = if (enabled) ripple(0xFFFFFFFF.toInt(), 13, 0xFFC8D8F0.toInt()) else ripple(COLOR_BLUE, 13)
        serviceButton.setTextColor(if (enabled) COLOR_BLUE else Color.WHITE)

        renderMode(strictModeCard, strictModeCheck, !convenience)
        renderMode(convenienceModeCard, convenienceModeCheck, convenience)
        deviceTitle.text = if (paired) "已配对的 Windows 电脑" else "尚未配对电脑"
        deviceSubtitle.text = if (paired) "加密挑战响应已启用" else "扫描电脑端二维码即可建立连接"
        deviceBadge.text = if (paired) "已配对" else "未配对"
        deviceBadge.setTextColor(if (paired) COLOR_GREEN else COLOR_MUTED)
        deviceBadge.background = shape(if (paired) COLOR_GREEN_SOFT else 0xFFF0F2F6.toInt(), 14)
        revokeButton.visibility = if (paired) View.VISIBLE else View.GONE
        keyBackend.text = if (paired) {
            try {
                "不可导出密钥  ·  安全 ${store.backend(true)}  ·  便捷 ${store.backend(false)}"
            } catch (_: Exception) {
                "不可导出密钥已由 Android Keystore 保护"
            }
        } else {
            "完成配对后，密钥将优先保存到 StrongBox 或 TEE"
        }
        if (!paired) pairingPanel.visibility = View.VISIBLE
        renderLogs()
    }

    private fun renderLogs() {
        logContainer.removeAllViews()
        val events = store.events().take(5)
        if (events.isEmpty()) {
            logContainer.addView(text("还没有安全事件", 13f, COLOR_MUTED).apply {
                gravity = Gravity.CENTER
                setPadding(0, dp(18), 0, dp(12))
            })
            return
        }
        val clock = SimpleDateFormat("HH:mm", Locale.CHINA)
        events.forEachIndexed { index, event ->
            if (index > 0) {
                logContainer.addView(View(this).apply { setBackgroundColor(0xFFF0F2F6.toInt()) }, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(1)).apply {
                    marginStart = dp(28)
                })
            }
            val row = LinearLayout(this).apply {
                orientation = LinearLayout.HORIZONTAL
                gravity = Gravity.CENTER_VERTICAL
                setPadding(dp(2), dp(9), dp(2), dp(9))
            }
            val eventColor = when (event.tone) {
                "error" -> COLOR_RED
                "warning" -> COLOR_ORANGE
                else -> if (event.message.contains("正常") || event.message.contains("完成")) COLOR_GREEN else COLOR_BLUE
            }
            row.addView(text("●", 13f, eventColor).apply { gravity = Gravity.CENTER }, LinearLayout.LayoutParams(dp(28), dp(38)))
            val copy = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
            copy.addView(text(event.message, 13f, COLOR_TEXT, true))
            copy.addView(text("蓝牙解锁服务", 11f, COLOR_MUTED).apply { setPadding(0, dp(2), 0, 0) })
            row.addView(copy, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
            row.addView(text(clock.format(Date(event.at)), 12f, COLOR_MUTED))
            logContainer.addView(row)
        }
    }

    private fun toggleService() {
        if (store.enabled()) {
            store.setEnabled(false)
            store.setRuntimeStatus("已停止")
            startService(Intent(this, UnlockService::class.java).setAction(UnlockService.ACTION_STOP))
            renderStatus()
            return
        }
        if (!store.isPaired()) {
            togglePairingPanel(true)
            toast("请先扫描电脑上的配对二维码")
            return
        }
        if (!hasBluetoothPermissions()) {
            requestPermissionsIfNeeded()
            toast("请允许附近设备权限")
            return
        }
        store.setEnabled(true)
        store.setRuntimeStatus("正在启动 BLE 广播")
        store.addEvent("正在手动启动蓝牙钥匙", dedupeWindowMs = 0)
        UnlockService.start(this)
        renderStatus()
    }

    private fun switchMode(convenience: Boolean) = requireUnlocked {
        if (convenience && !store.convenienceAllowed()) {
            AlertDialog.Builder(this)
                .setTitle("启用便捷模式？")
                .setMessage("便捷模式允许手机在锁屏时响应附近电脑。持有锁屏手机的人可能解锁已配对电脑。")
                .setNegativeButton("取消", null)
                .setPositiveButton("仍要启用") { _, _ -> applyMode(true) }
                .show()
        } else {
            applyMode(convenience)
        }
    }

    private fun applyMode(convenience: Boolean) {
        store.setConvenienceAllowed(convenience)
        store.addEvent(if (convenience) "已切换到便捷模式" else "已切换到安全模式", if (convenience) "warning" else "info", 0)
        refreshService()
        renderStatus()
    }

    private fun savePairing() {
        try {
            val pairing = Protocol.parsePairingUri(uriInput.text.toString().trim())
            store.ensureSigningKeys()
            store.savePairing(pairing)
            pairing.secret.fill(0)
            store.setRuntimeStatus("正在等待电脑完成配对")
            store.addEvent("已接收新的电脑配对请求", dedupeWindowMs = 0)
            UnlockService.start(this)
            uriInput.text.clear()
            renderStatus()
            toast("配对请求已保存，请保持手机靠近电脑")
        } catch (error: Exception) {
            store.addEvent("配对信息校验失败", "warning", 10_000)
            renderLogs()
            toast(error.message ?: "配对失败")
        }
    }

    private fun confirmRevoke() = requireUnlocked {
        AlertDialog.Builder(this)
            .setTitle("撤销这台电脑？")
            .setMessage("手机将删除电脑配对、presence key 和签名密钥。之后需要重新扫码才能使用。")
            .setNegativeButton("取消", null)
            .setPositiveButton("撤销设备") { _, _ ->
                store.clearPairing()
                store.addEvent("已撤销电脑配对", "warning", 0)
                startService(Intent(this, UnlockService::class.java).setAction(UnlockService.ACTION_STOP))
                togglePairingPanel(true)
                renderStatus()
            }
            .show()
    }

    private fun refreshService() {
        if (store.enabled()) startForegroundService(Intent(this, UnlockService::class.java).setAction(UnlockService.ACTION_REFRESH))
    }

    private fun showPairingUri(uri: String) {
        uriInput.setText(uri)
        togglePairingPanel(true)
    }

    private fun togglePairingPanel(forceOpen: Boolean? = null) {
        val open = forceOpen ?: (pairingPanel.visibility != View.VISIBLE)
        pairingPanel.visibility = if (open) View.VISIBLE else View.GONE
        if (open) scrollView.post { scrollView.smoothScrollTo(0, deviceCard.top) }
    }

    private fun requireUnlocked(action: () -> Unit) {
        if (getSystemService(KeyguardManager::class.java).isDeviceLocked) toast("请先解锁手机") else action()
    }

    private fun requestPermissionsIfNeeded() {
        val permissions = listOf(
            Manifest.permission.BLUETOOTH_ADVERTISE,
            Manifest.permission.BLUETOOTH_CONNECT,
            Manifest.permission.POST_NOTIFICATIONS,
        ).filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
        if (permissions.isNotEmpty()) requestPermissions(permissions.toTypedArray(), PERMISSION_REQUEST)
    }

    private fun hasBluetoothPermissions(): Boolean =
        checkSelfPermission(Manifest.permission.BLUETOOTH_ADVERTISE) == PackageManager.PERMISSION_GRANTED &&
            checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED

    private fun modeRow(badge: String, title: String, subtitle: String, onClick: () -> Unit): Pair<LinearLayout, TextView> {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(13), 0, dp(13), 0)
            isClickable = true
            isFocusable = true
            setOnClickListener { onClick() }
        }
        row.addView(text(badge, 12f, COLOR_BLUE, true).apply {
            gravity = Gravity.CENTER
            background = shape(COLOR_BLUE_SOFT, 12)
        }, LinearLayout.LayoutParams(dp(40), dp(40)))
        val copy = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(12), 0, 0, 0)
        }
        copy.addView(text(title, 15f, COLOR_TEXT, true))
        copy.addView(text(subtitle, 12f, COLOR_MUTED).apply { setPadding(0, dp(3), 0, 0) })
        row.addView(copy, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        val check = text("", 15f, Color.WHITE, true).apply { gravity = Gravity.CENTER }
        row.addView(check, LinearLayout.LayoutParams(dp(28), dp(28)))
        return row to check
    }

    private fun renderMode(card: LinearLayout, check: TextView, selected: Boolean) {
        card.background = ripple(if (selected) COLOR_BLUE_SOFT else 0xFFFBFCFF.toInt(), 15, if (selected) 0xFF8FB3FF.toInt() else COLOR_BORDER)
        check.text = if (selected) "✓" else ""
        check.background = shape(if (selected) COLOR_BLUE else 0xFFE8ECF3.toInt(), 14)
    }

    private fun sectionCard() = LinearLayout(this).apply {
        orientation = LinearLayout.VERTICAL
        setPadding(dp(18), dp(18), dp(18), dp(18))
        background = shape(Color.WHITE, 20, 0xFFE4EAF2.toInt())
        elevation = dp(1).toFloat()
    }

    private fun sectionTitle(title: String, subtitle: String) = LinearLayout(this).apply {
        orientation = LinearLayout.VERTICAL
        addView(text(title, 18f, COLOR_TEXT, true))
        addView(text(subtitle, 12f, COLOR_MUTED).apply { setPadding(0, dp(4), 0, 0) })
    }

    private fun actionButton(label: String, primary: Boolean, onClick: () -> Unit) = text(
        label,
        14f,
        if (primary) Color.WHITE else COLOR_BLUE,
        true,
    ).apply {
        gravity = Gravity.CENTER
        isClickable = true
        isFocusable = true
        background = if (primary) ripple(COLOR_BLUE, 13) else ripple(Color.WHITE, 13, 0xFFC8D8F0.toInt())
        setOnClickListener { onClick() }
    }

    private fun chip(label: String) = text(label, 11f, COLOR_MUTED, true).apply {
        gravity = Gravity.CENTER
        background = shape(0xFFF0F3F8.toInt(), 17)
    }

    private fun styleChip(view: TextView, label: String, active: Boolean) {
        view.text = label
        view.setTextColor(if (active) COLOR_GREEN else COLOR_MUTED)
        view.background = shape(if (active) COLOR_GREEN_SOFT else 0xFFF0F3F8.toInt(), 17)
    }

    private fun text(value: String, size: Float, color: Int, bold: Boolean = false) = TextView(this).apply {
        text = value
        textSize = size
        setTextColor(color)
        includeFontPadding = false
        typeface = Typeface.create("sans-serif", if (bold) Typeface.BOLD else Typeface.NORMAL)
    }

    private fun verticalParams(top: Int = 0, height: Int = ViewGroup.LayoutParams.WRAP_CONTENT) =
        LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, if (height > 0) dp(height) else height).apply { topMargin = dp(top) }

    private fun shape(fill: Int, radius: Int, stroke: Int? = null) = GradientDrawable().apply {
        shape = GradientDrawable.RECTANGLE
        setColor(fill)
        cornerRadius = dp(radius).toFloat()
        if (stroke != null) setStroke(dp(1), stroke)
    }

    private fun gradient(colors: IntArray, radius: Int) = GradientDrawable(GradientDrawable.Orientation.TL_BR, colors).apply {
        cornerRadius = dp(radius).toFloat()
    }

    private fun ripple(fill: Int, radius: Int, stroke: Int? = null) = RippleDrawable(
        ColorStateList.valueOf(0x1F3978F6),
        shape(fill, radius, stroke),
        null,
    )

    private fun dp(value: Int) = (value * resources.displayMetrics.density + .5f).toInt()

    private fun toast(message: String) = Toast.makeText(this, message, Toast.LENGTH_LONG).show()

    companion object {
        private const val PERMISSION_REQUEST = 42
        private const val COLOR_PAGE = 0xFFF5F8FD.toInt()
        private const val COLOR_TEXT = 0xFF18233A.toInt()
        private const val COLOR_SUBTEXT = 0xFF5D6A7E.toInt()
        private const val COLOR_MUTED = 0xFF8793A6.toInt()
        private const val COLOR_BORDER = 0xFFE2E8F1.toInt()
        private const val COLOR_BLUE = 0xFF3978F6.toInt()
        private const val COLOR_BLUE_SOFT = 0xFFEEF4FF.toInt()
        private const val COLOR_GREEN = 0xFF2B9B72.toInt()
        private const val COLOR_GREEN_SOFT = 0xFFE9F8F2.toInt()
        private const val COLOR_ORANGE = 0xFFE18A32.toInt()
        private const val COLOR_RED = 0xFFD95663.toInt()
    }
}
