package com.singu.proximityunlock

import android.Manifest
import android.app.Activity
import android.app.KeyguardManager
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.graphics.Color
import android.os.Bundle
import android.provider.Settings
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast

class MainActivity : Activity() {
    private lateinit var store: SecureStore
    private lateinit var uriInput: EditText
    private lateinit var status: TextView
	private var statusReceiverRegistered = false
	private val statusReceiver = object : BroadcastReceiver() {
		override fun onReceive(context: Context?, intent: Intent?) {
			renderStatus()
		}
	}

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        store = SecureStore(this)
        buildUi()
        intent?.dataString?.let { uriInput.setText(it) }
        requestPermissionsIfNeeded()
        renderStatus()
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        intent.dataString?.let { uriInput.setText(it) }
    }

	override fun onStart() {
		super.onStart()
		registerReceiver(
			statusReceiver,
			IntentFilter(UnlockService.ACTION_STATUS_CHANGED),
			RECEIVER_NOT_EXPORTED,
		)
		statusReceiverRegistered = true
		if (store.enabled() &&
			checkSelfPermission(Manifest.permission.BLUETOOTH_ADVERTISE) == PackageManager.PERMISSION_GRANTED &&
			checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED
		) {
			UnlockService.start(this)
		}
		renderStatus()
	}

	override fun onStop() {
		if (statusReceiverRegistered) {
			unregisterReceiver(statusReceiver)
			statusReceiverRegistered = false
		}
		super.onStop()
	}

    private fun buildUi() {
        val density = resources.displayMetrics.density
        fun dp(value: Int) = (value * density).toInt()
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(24), dp(32), dp(24), dp(32))
            setBackgroundColor(Color.rgb(248, 250, 252))
        }
        content.addView(TextView(this).apply {
            text = "Proximity Unlock"
            textSize = 28f
            setTextColor(Color.rgb(15, 23, 42))
        })
        content.addView(TextView(this).apply {
            text = "Android 15 蓝牙钥匙 · 私钥不可导出"
            textSize = 15f
            setTextColor(Color.rgb(71, 85, 105))
            setPadding(0, dp(6), 0, dp(20))
        })
        status = TextView(this).apply {
            textSize = 16f
            setTextColor(Color.rgb(30, 41, 59))
            setPadding(0, 0, 0, dp(20))
        }
        content.addView(status)
        uriInput = EditText(this).apply {
            hint = "扫描电脑二维码，或粘贴 proximityunlock://pair…"
            minLines = 3
            maxLines = 6
        }
        content.addView(uriInput, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT)
        content.addView(button("保存配对并启动") { savePairing() })
        content.addView(button("安全模式（手机必须解锁）") {
            requireUnlocked {
                store.setConvenienceAllowed(false)
                refreshService()
                renderStatus()
            }
        })
        content.addView(button("便捷模式（锁屏也允许）") {
            requireUnlocked {
                store.setConvenienceAllowed(true)
                refreshService()
                renderStatus()
            }
        })
        content.addView(button("停止蓝牙钥匙") {
            store.setEnabled(false)
            startService(Intent(this, UnlockService::class.java).setAction(UnlockService.ACTION_STOP))
            renderStatus()
        })
        content.addView(button("撤销此电脑") {
            requireUnlocked {
                store.clearPairing()
                startService(Intent(this, UnlockService::class.java).setAction(UnlockService.ACTION_STOP))
                renderStatus()
            }
        })
        setContentView(ScrollView(this).apply { addView(content) })
    }

    private fun button(label: String, action: () -> Unit) = Button(this).apply {
        text = label
        isAllCaps = false
        setOnClickListener { action() }
    }

    private fun savePairing() {
        try {
            val pairing = Protocol.parsePairingUri(uriInput.text.toString())
            store.ensureSigningKeys()
            store.savePairing(pairing)
            pairing.secret.fill(0)
            store.setEnabled(true)
            UnlockService.start(this)
            uriInput.text.clear()
            renderStatus()
            toast("配对等待已启动，请保持本页或常驻服务运行")
        } catch (error: Exception) {
            toast(error.message ?: "配对失败")
        }
    }

    private fun renderStatus() {
        val paired = store.isPaired()
        val mode = if (store.convenienceAllowed()) "便捷模式" else "安全模式"
        val backend = try {
            store.ensureSigningKeys()
            "严格密钥 ${store.backend(true)} / 便捷密钥 ${store.backend(false)}"
        } catch (_: Exception) { "密钥尚未创建" }
		status.text = "状态：${if (store.enabled()) "运行中" else "已停止"}\n蓝牙：${store.runtimeStatus()}\n配对：${if (paired) "已完成" else "未完成"}\n模式：$mode\n$backend"
    }

    private fun refreshService() {
        if (store.enabled()) startForegroundService(Intent(this, UnlockService::class.java).setAction(UnlockService.ACTION_REFRESH))
    }

    private fun requireUnlocked(action: () -> Unit) {
        if (getSystemService(KeyguardManager::class.java).isDeviceLocked) {
            toast("请先解锁手机")
        } else action()
    }

    private fun requestPermissionsIfNeeded() {
        val permissions = listOf(
            Manifest.permission.BLUETOOTH_ADVERTISE,
            Manifest.permission.BLUETOOTH_CONNECT,
            Manifest.permission.POST_NOTIFICATIONS,
        ).filter { checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED }
        if (permissions.isNotEmpty()) requestPermissions(permissions.toTypedArray(), 42)
    }

    private fun toast(message: String) = Toast.makeText(this, message, Toast.LENGTH_LONG).show()
}
