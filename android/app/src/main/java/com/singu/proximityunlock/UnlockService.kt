package com.singu.proximityunlock

import android.Manifest
import android.annotation.SuppressLint
import android.app.KeyguardManager
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothGattDescriptor
import android.bluetooth.BluetoothGattServer
import android.bluetooth.BluetoothGattServerCallback
import android.bluetooth.BluetoothGattService
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.bluetooth.BluetoothStatusCodes
import android.bluetooth.le.AdvertiseCallback
import android.bluetooth.le.AdvertiseData
import android.bluetooth.le.AdvertiseSettings
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.drawable.Icon
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.util.Log
import java.util.concurrent.Executors

class UnlockService : Service() {
    private lateinit var store: SecureStore
    private lateinit var bluetoothManager: BluetoothManager
    private var gattServer: BluetoothGattServer? = null
	private var gattReady = false
    private val handler = Handler(Looper.getMainLooper())
    private val worker = Executors.newSingleThreadExecutor()
	private val reassemblers = mutableMapOf<Pair<String, java.util.UUID>, Protocol.Reassembler>()
    private val subscribed = mutableSetOf<Pair<String, java.util.UUID>>()
    private var advertiserCallback: AdvertiseCallback? = null

    override fun onCreate() {
        super.onCreate()
        store = SecureStore(this)
        bluetoothManager = getSystemService(BluetoothManager::class.java)
        createChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_STOP -> {
                store.setEnabled(false)
				store.setRuntimeStatus("已停止")
				store.addEvent("蓝牙钥匙已停止", "warning", 0)
                stopRuntime()
				notifyStatusChanged()
				stopForeground(STOP_FOREGROUND_REMOVE)
				Log.i(TAG, "BLE key stopped")
                stopSelf()
                return START_NOT_STICKY
            }
            ACTION_REFRESH -> if (store.enabled()) restartAdvertising()
        }
        if (!store.enabled()) return START_NOT_STICKY
        startForeground(NOTIFICATION_ID, notification())
		Log.i(TAG, "BLE key foreground service started")
		store.addEvent("蓝牙前台服务已启动", dedupeWindowMs = 60_000)
        if (hasBluetoothPermission()) startRuntime()
        return START_STICKY
    }

    override fun onDestroy() {
        stopRuntime()
        worker.shutdownNow()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun startRuntime() {
        try {
			if (bluetoothManager.adapter?.isMultipleAdvertisementSupported != true) {
				store.setRuntimeStatus("此手机不支持 BLE Peripheral 广播")
				store.addEvent("手机不支持 BLE Peripheral 广播", "error", 0)
				refreshNotification()
				stopSelf()
				return
			}
            store.ensureSigningKeys()
			if (gattServer == null) {
				store.setRuntimeStatus("正在初始化 GATT 服务")
				refreshNotification()
				createGattServer()
			} else if (gattReady) {
				restartAdvertising()
			}
        } catch (_: Exception) {
			store.setRuntimeStatus("蓝牙或硬件密钥初始化失败")
			store.addEvent("蓝牙或硬件密钥初始化失败", "error", 0)
			refreshNotification()
            stopSelf()
        }
    }

    @SuppressLint("MissingPermission")
    private fun createGattServer() {
        if (!hasBluetoothPermission()) {
            handleMissingBluetoothPermission()
            return
        }
        gattReady = false
        try {
            gattServer = bluetoothManager.openGattServer(this, callback).also { server ->
                val service = BluetoothGattService(Protocol.SERVICE_UUID, BluetoothGattService.SERVICE_TYPE_PRIMARY)
                val challenge = BluetoothGattCharacteristic(
                    Protocol.CHALLENGE_UUID,
                    BluetoothGattCharacteristic.PROPERTY_WRITE,
                    BluetoothGattCharacteristic.PERMISSION_WRITE,
                )
                val response = BluetoothGattCharacteristic(
                    Protocol.RESPONSE_UUID,
                    BluetoothGattCharacteristic.PROPERTY_NOTIFY,
                    BluetoothGattCharacteristic.PERMISSION_READ,
                ).apply { addDescriptor(cccDescriptor()) }
                val pairing = BluetoothGattCharacteristic(
                    Protocol.PAIRING_UUID,
                    BluetoothGattCharacteristic.PROPERTY_WRITE or BluetoothGattCharacteristic.PROPERTY_NOTIFY,
                    BluetoothGattCharacteristic.PERMISSION_WRITE or BluetoothGattCharacteristic.PERMISSION_READ,
                ).apply { addDescriptor(cccDescriptor()) }
                service.addCharacteristic(challenge)
                service.addCharacteristic(response)
                service.addCharacteristic(pairing)
                server.addService(service)
            }
        } catch (_: SecurityException) {
            handleMissingBluetoothPermission()
        }
    }

    private fun cccDescriptor() = BluetoothGattDescriptor(
        Protocol.CCC_UUID,
        BluetoothGattDescriptor.PERMISSION_READ or BluetoothGattDescriptor.PERMISSION_WRITE,
    )

    private val callback = object : BluetoothGattServerCallback() {
		override fun onServiceAdded(status: Int, service: BluetoothGattService) {
			if (service.uuid != Protocol.SERVICE_UUID) return
			gattReady = status == BluetoothGatt.GATT_SUCCESS
			if (gattReady && store.enabled()) {
				Log.i(TAG, "GATT service ready")
				store.addEvent("GATT 安全服务已就绪", dedupeWindowMs = 60_000)
				handler.post { restartAdvertising() }
			} else {
				Log.w(TAG, "GATT service initialization failed: $status")
				store.setRuntimeStatus("GATT 服务初始化失败（错误码 $status）")
				store.addEvent("GATT 服务初始化失败（错误码 $status）", "error", 0)
				refreshNotification()
				notifyStatusChanged()
			}
		}

        override fun onConnectionStateChange(device: BluetoothDevice, status: Int, newState: Int) {
            if (newState != BluetoothProfile.STATE_CONNECTED) {
                subscribed.removeAll { it.first == device.address }
				reassemblers.keys.removeAll { it.first == device.address }
            }
        }

        override fun onDescriptorWriteRequest(
            device: BluetoothDevice,
            requestId: Int,
            descriptor: BluetoothGattDescriptor,
            preparedWrite: Boolean,
            responseNeeded: Boolean,
            offset: Int,
            value: ByteArray,
        ) {
            if (descriptor.uuid == Protocol.CCC_UUID && value.contentEquals(BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE)) {
                subscribed += device.address to descriptor.characteristic.uuid
            }
            if (responseNeeded) sendGattResponse(device, requestId, offset)
        }

        override fun onCharacteristicWriteRequest(
            device: BluetoothDevice,
            requestId: Int,
            characteristic: BluetoothGattCharacteristic,
            preparedWrite: Boolean,
            responseNeeded: Boolean,
            offset: Int,
            value: ByteArray,
        ) {
            if (responseNeeded) sendGattResponse(device, requestId, offset)
			val reassemblerKey = device.address to characteristic.uuid
			val reassembler = reassemblers.getOrPut(reassemblerKey) { Protocol.Reassembler() }
            try {
                val complete = reassembler.add(value) ?: return
                worker.submit {
                    when (complete.first) {
                        Protocol.MESSAGE_CHALLENGE -> processChallenge(device, complete.second)
                        Protocol.MESSAGE_PAIRING -> processPairing(device, complete.second)
                    }
                }
            } catch (_: Exception) {
				reassemblers.remove(reassemblerKey)
            }
        }
    }

    private fun processChallenge(device: BluetoothDevice, payload: ByteArray) {
        try {
            val pcId = store.pcId() ?: return
            val phoneId = store.phoneId
            val pcPublic = Protocol.publicKey(store.pcPublic() ?: return)
            val challenge = Protocol.parseChallenge(payload, pcPublic, pcId, phoneId)
            val strict = challenge.mode == Protocol.MODE_STRICT
            if (!strict && !store.convenienceAllowed()) {
				store.addEvent("已拒绝未启用的便捷模式认证", "warning", 60_000)
				notifyStatusChanged()
				return
			}
            if (strict && getSystemService(KeyguardManager::class.java).isDeviceLocked) {
				store.addEvent("安全模式：手机锁屏，已拒绝认证", "warning", 60_000)
				notifyStatusChanged()
				return
			}
            val counter = store.nextCounter()
            val response = Protocol.response(challenge, counter) { store.sign(strict, it) }
            notifyFragments(device, Protocol.RESPONSE_UUID, Protocol.MESSAGE_CHALLENGE, response)
			store.addEvent("已完成电脑认证挑战", dedupeWindowMs = 60_000)
			notifyStatusChanged()
		} catch (_: Exception) {
			store.addEvent("认证挑战校验失败", "warning", 60_000)
			notifyStatusChanged()
            // Fail closed and intentionally return no protocol oracle.
        }
    }

    private fun processPairing(device: BluetoothDevice, payload: ByteArray) {
        val pending = store.pendingPairing() ?: return
        try {
            val challenge = Protocol.parsePairingChallenge(payload, pending.secret)
            if (!challenge.pcId.contentEquals(pending.pcId)) return
            val phoneId = store.phoneId
            val response = Protocol.pairingResponse(
                challenge,
                phoneId,
                store.publicSec1(true),
                store.publicSec1(false),
                pending.secret,
            )
            notifyFragments(device, Protocol.PAIRING_UUID, Protocol.MESSAGE_PAIRING, response)
            val presence = Protocol.presenceKey(pending.secret, pending.pcId, phoneId)
            store.completePairing(presence)
            presence.fill(0)
			store.addEvent("电脑配对已安全完成", dedupeWindowMs = 0)
			handler.post {
				restartAdvertising()
				notifyStatusChanged()
			}
        } catch (_: Exception) {
            // Pairing stays pending until expiry.
        } finally {
            pending.secret.fill(0)
        }
    }

    @SuppressLint("MissingPermission")
    private fun notifyFragments(device: BluetoothDevice, uuid: java.util.UUID, type: Byte, payload: ByteArray) {
        if (!hasBluetoothPermission()) {
            handleMissingBluetoothPermission()
            return
        }
        if (device.address to uuid !in subscribed) return
        val characteristic = gattServer?.getService(Protocol.SERVICE_UUID)?.getCharacteristic(uuid) ?: return
        try {
            for (fragment in Protocol.fragment(type, payload)) {
                val result = gattServer?.notifyCharacteristicChanged(device, characteristic, false, fragment)
                if (result != BluetoothStatusCodes.SUCCESS) return
                Thread.sleep(25)
            }
        } catch (_: SecurityException) {
            handleMissingBluetoothPermission()
        }
    }

    @SuppressLint("MissingPermission")
    private fun restartAdvertising() {
        if (!hasBluetoothPermission()) {
            handleMissingBluetoothPermission()
            return
        }
        val advertiser = try {
            bluetoothManager.adapter?.bluetoothLeAdvertiser
        } catch (_: SecurityException) {
            handleMissingBluetoothPermission()
            null
        } ?: return
        try {
            advertiserCallback?.let { advertiser.stopAdvertising(it) }
        } catch (_: SecurityException) {
            handleMissingBluetoothPermission()
            return
        }
		advertiserCallback = null
        val pending = store.pendingPairing()
        val data = when {
            pending != null -> Protocol.pairingAdvertisement(pending.secret, pending.pcId).also { pending.secret.fill(0) }
            else -> store.presenceKey()?.let { key -> Protocol.rollingAdvertisement(key).also { key.fill(0) } }
        }
		if (data == null) {
			store.setRuntimeStatus("等待新的配对二维码")
			store.addEvent("正在等待电脑配对二维码", dedupeWindowMs = 60_000)
			refreshNotification()
			notifyStatusChanged()
			return
		}
        val settings = AdvertiseSettings.Builder()
            .setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_LOW_LATENCY)
            .setTxPowerLevel(AdvertiseSettings.ADVERTISE_TX_POWER_MEDIUM)
            .setConnectable(true)
            .build()
        val advertiseData = AdvertiseData.Builder()
			.addManufacturerData(Protocol.ADVERTISEMENT_COMPANY_ID, data)
            .setIncludeDeviceName(false)
            .build()
		store.setRuntimeStatus("正在启动 BLE 广播")
		advertiserCallback = object : AdvertiseCallback() {
			override fun onStartSuccess(settingsInEffect: AdvertiseSettings) {
				Log.i(TAG, "BLE advertising started")
				store.setRuntimeStatus("BLE 广播运行中")
				store.addEvent("BLE 广播运行正常", dedupeWindowMs = 5 * 60_000)
				refreshNotification()
				notifyStatusChanged()
			}

			override fun onStartFailure(errorCode: Int) {
				Log.w(TAG, "BLE advertising failed: $errorCode")
				store.setRuntimeStatus("BLE 广播失败（错误码 $errorCode）")
				store.addEvent("BLE 广播失败（错误码 $errorCode）", "error", 0)
				refreshNotification()
				notifyStatusChanged()
			}
		}
        try {
            advertiser.startAdvertising(settings, advertiseData, advertiserCallback)
        } catch (_: SecurityException) {
            handleMissingBluetoothPermission()
            return
        }
        handler.removeCallbacks(refreshAdvertisement)
        handler.postDelayed(refreshAdvertisement, 30_000)
    }

    private val refreshAdvertisement = Runnable { if (store.enabled()) restartAdvertising() }

    @SuppressLint("MissingPermission")
    private fun stopRuntime() {
        handler.removeCallbacksAndMessages(null)
        if (hasBluetoothPermission()) {
            try {
                advertiserCallback?.let { bluetoothManager.adapter?.bluetoothLeAdvertiser?.stopAdvertising(it) }
                gattServer?.close()
            } catch (_: SecurityException) {
                Log.w(TAG, "Bluetooth permission was revoked while stopping")
            }
        }
        advertiserCallback = null
        gattServer = null
		gattReady = false
    }

	private fun refreshNotification() {
		getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, notification())
	}

	private fun notifyStatusChanged() {
		sendBroadcast(Intent(ACTION_STATUS_CHANGED).setPackage(packageName))
	}

    @SuppressLint("MissingPermission")
    private fun sendGattResponse(device: BluetoothDevice, requestId: Int, offset: Int) {
        if (!hasBluetoothPermission()) {
            handleMissingBluetoothPermission()
            return
        }
        try {
            gattServer?.sendResponse(device, requestId, BluetoothGatt.GATT_SUCCESS, offset, null)
        } catch (_: SecurityException) {
            handleMissingBluetoothPermission()
        }
    }

    private fun handleMissingBluetoothPermission() {
        store.setRuntimeStatus("缺少附近设备权限")
        store.addEvent("附近设备权限不可用，蓝牙钥匙已停止", "warning", 60_000)
        refreshNotification()
        notifyStatusChanged()
        stopSelf()
    }

    private fun notification(): android.app.Notification {
        val openIntent = PendingIntent.getActivity(
            this,
            1,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val stopIntent = PendingIntent.getService(
            this,
            2,
            Intent(this, UnlockService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
		val mode = if (store.convenienceAllowed()) "便捷模式" else "安全模式"
        return android.app.Notification.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_lock_idle_lock)
            .setContentTitle("蓝牙解锁正在运行")
			.setContentText("$mode · ${store.runtimeStatus()}")
            .setContentIntent(openIntent)
            .setOngoing(true)
			.addAction(android.app.Notification.Action.Builder(
				Icon.createWithResource(this, android.R.drawable.ic_media_pause),
				"停止",
				stopIntent,
			).build())
            .build()
    }

    private fun createChannel() {
        getSystemService(NotificationManager::class.java).createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "蓝牙钥匙", NotificationManager.IMPORTANCE_LOW),
        )
    }

    private fun hasBluetoothPermission(): Boolean =
        checkSelfPermission(Manifest.permission.BLUETOOTH_ADVERTISE) == PackageManager.PERMISSION_GRANTED &&
            checkSelfPermission(Manifest.permission.BLUETOOTH_CONNECT) == PackageManager.PERMISSION_GRANTED

    companion object {
        const val ACTION_START = "com.singu.proximityunlock.START"
        const val ACTION_STOP = "com.singu.proximityunlock.STOP"
        const val ACTION_REFRESH = "com.singu.proximityunlock.REFRESH"
		const val ACTION_STATUS_CHANGED = "com.singu.proximityunlock.STATUS_CHANGED"
        private const val CHANNEL_ID = "proximity_unlock"
        private const val NOTIFICATION_ID = 1001
		private const val TAG = "ProximityUnlock"

        fun start(context: Context) {
            context.startForegroundService(Intent(context, UnlockService::class.java).setAction(ACTION_START))
        }
    }
}
