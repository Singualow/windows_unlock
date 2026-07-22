package com.singu.proximityunlock

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import java.nio.ByteBuffer
import java.security.KeyFactory
import java.security.KeyPair
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class SecureStore(private val context: Context) {
    private val prefs = context.getSharedPreferences("proximity_unlock", Context.MODE_PRIVATE)
    private val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    val phoneId: ByteArray
        get() {
            val existing = prefs.getString("phone_id", null)
            if (existing != null) return decode(existing)
            val created = ByteArray(16).also { java.security.SecureRandom().nextBytes(it) }
            prefs.edit().putString("phone_id", encode(created)).apply()
            return created
        }

    fun ensureSigningKeys(): Pair<KeyPair, KeyPair> {
        val keys = keyPair(STRICT_ALIAS, true) to keyPair(RELAXED_ALIAS, false)
        require(backend(true) != "software" && backend(false) != "software") {
            "Android hardware-backed Keystore (StrongBox or TEE) is required"
        }
        return keys
    }

    fun sign(strict: Boolean, data: ByteArray): ByteArray {
        val alias = if (strict) STRICT_ALIAS else RELAXED_ALIAS
        val privateKey = keyStore.getKey(alias, null) as PrivateKey
        return Protocol.derToRaw(Signature.getInstance("SHA256withECDSA").run {
            initSign(privateKey)
            update(data)
            sign()
        })
    }

    fun publicSec1(strict: Boolean): ByteArray {
        val alias = if (strict) STRICT_ALIAS else RELAXED_ALIAS
        val public = keyStore.getCertificate(alias).publicKey as ECPublicKey
        return byteArrayOf(4) + unsigned32(public.w.affineX.toByteArray()) + unsigned32(public.w.affineY.toByteArray())
    }

    fun backend(strict: Boolean): String {
        val alias = if (strict) STRICT_ALIAS else RELAXED_ALIAS
        val privateKey = keyStore.getKey(alias, null) as PrivateKey
        val info = KeyFactory.getInstance(privateKey.algorithm, "AndroidKeyStore").getKeySpec(privateKey, KeyInfo::class.java)
        return when {
            info.securityLevel == KeyProperties.SECURITY_LEVEL_STRONGBOX -> "StrongBox"
            info.securityLevel == KeyProperties.SECURITY_LEVEL_TRUSTED_ENVIRONMENT -> "TEE"
            else -> "software"
        }
    }

    fun savePairing(pairing: Protocol.PairingUri) {
        prefs.edit()
            .putString("pc_id", encode(pairing.pcId))
            .putString("pc_public", encode(pairing.pcPublicKey))
            .putString("pair_secret", encrypt(pairing.secret))
            .putLong("pair_expiry", pairing.expiresAtSeconds)
			.remove("presence_key")
			.remove("counter")
            .putBoolean("enabled", true)
            .apply()
    }

    fun pendingPairing(): Protocol.PairingUri? {
		val secret = prefs.getString("pair_secret", null) ?: return null
        val expiry = prefs.getLong("pair_expiry", 0)
        if (expiry <= System.currentTimeMillis() / 1000) {
			val editor = prefs.edit().remove("pair_secret").remove("pair_expiry")
			if (!prefs.contains("presence_key")) editor.remove("pc_id").remove("pc_public")
			editor.apply()
			return null
		}
        val pc = prefs.getString("pc_id", null) ?: return null
        val pub = prefs.getString("pc_public", null) ?: return null
        return Protocol.PairingUri(decode(pc), decode(pub), decrypt(secret), expiry)
    }

    fun completePairing(presenceKey: ByteArray) {
        prefs.edit()
            .putString("presence_key", encrypt(presenceKey))
            .remove("pair_secret")
            .remove("pair_expiry")
            .apply()
    }

    fun pcId(): ByteArray? = prefs.getString("pc_id", null)?.let(::decode)
    fun pcPublic(): ByteArray? = prefs.getString("pc_public", null)?.let(::decode)
    fun presenceKey(): ByteArray? = prefs.getString("presence_key", null)?.let(::decrypt)
    fun isPaired(): Boolean = presenceKey() != null && pcId() != null && pcPublic() != null

    @Synchronized
    fun nextCounter(): Long {
        val next = prefs.getLong("counter", 0) + 1
        prefs.edit().putLong("counter", next).commit()
        return next
    }

    fun setConvenienceAllowed(allowed: Boolean) = prefs.edit().putBoolean("allow_convenience", allowed).apply()
    fun convenienceAllowed(): Boolean = prefs.getBoolean("allow_convenience", false)
    fun setEnabled(enabled: Boolean) = prefs.edit().putBoolean("enabled", enabled).apply()
    fun enabled(): Boolean = prefs.getBoolean("enabled", false)
	fun setRuntimeStatus(status: String) = prefs.edit().putString("runtime_status", status).apply()
	fun runtimeStatus(): String = prefs.getString("runtime_status", "尚未启动") ?: "尚未启动"

    data class SafeEvent(val at: Long, val message: String, val tone: String)

    @Synchronized
    fun addEvent(message: String, tone: String = "info", dedupeWindowMs: Long = 30_000) {
        val now = System.currentTimeMillis()
        val current = events().toMutableList()
        if (dedupeWindowMs > 0 && current.any { it.message == message && now - it.at < dedupeWindowMs }) return
        current.add(0, SafeEvent(now, message.take(80), tone))
        val encoded = JSONArray()
        current.take(MAX_SAFE_EVENTS).forEach { event ->
            encoded.put(JSONObject().put("at", event.at).put("message", event.message).put("tone", event.tone))
        }
        prefs.edit().putString("safe_events", encoded.toString()).apply()
    }

    @Synchronized
    fun events(): List<SafeEvent> = try {
        val source = JSONArray(prefs.getString("safe_events", "[]") ?: "[]")
        buildList {
            for (index in 0 until source.length()) {
                val item = source.optJSONObject(index) ?: continue
                val message = item.optString("message").takeIf { it.isNotBlank() } ?: continue
                add(SafeEvent(item.optLong("at"), message, item.optString("tone", "info")))
            }
        }
    } catch (_: Exception) {
        emptyList()
    }

    fun clearEvents() = prefs.edit().remove("safe_events").apply()

    fun clearPairing() {
		prefs.edit().remove("pc_id").remove("pc_public").remove("pair_secret").remove("pair_expiry")
			.remove("presence_key").remove("counter").remove("runtime_status").putBoolean("enabled", false).commit()
		keyStore.deleteEntry(STRICT_ALIAS)
		keyStore.deleteEntry(RELAXED_ALIAS)
		keyStore.deleteEntry(ENCRYPTION_ALIAS)
    }

    private fun keyPair(alias: String, strict: Boolean): KeyPair {
        if (keyStore.containsAlias(alias)) {
            return KeyPair(keyStore.getCertificate(alias).publicKey, keyStore.getKey(alias, null) as PrivateKey)
        }
        fun generate(strongBox: Boolean): KeyPair {
            val spec = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY)
                .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                .setDigests(KeyProperties.DIGEST_SHA256)
                .setUnlockedDeviceRequired(strict)
                .setIsStrongBoxBacked(strongBox)
                .build()
            return KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore").run {
                initialize(spec)
                generateKeyPair()
            }
        }
		return try {
			generate(true)
		} catch (_: Exception) {
			if (keyStore.containsAlias(alias)) keyStore.deleteEntry(alias)
			generate(false)
		}
    }

    private fun encryptionKey(): SecretKey {
        if (keyStore.containsAlias(ENCRYPTION_ALIAS)) return keyStore.getKey(ENCRYPTION_ALIAS, null) as SecretKey
        val spec = KeyGenParameterSpec.Builder(ENCRYPTION_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .build()
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").run {
            init(spec)
            generateKey()
        }
    }

    private fun encrypt(data: ByteArray): String {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, encryptionKey())
        val output = cipher.doFinal(data)
        return encode(ByteBuffer.allocate(4 + cipher.iv.size + output.size).putInt(cipher.iv.size).put(cipher.iv).put(output).array())
    }

    private fun decrypt(encoded: String): ByteArray {
        val buffer = ByteBuffer.wrap(decode(encoded))
        val iv = ByteArray(buffer.int).also { buffer.get(it) }
        val ciphertext = ByteArray(buffer.remaining()).also { buffer.get(it) }
        return Cipher.getInstance("AES/GCM/NoPadding").run {
            init(Cipher.DECRYPT_MODE, encryptionKey(), GCMParameterSpec(128, iv))
            doFinal(ciphertext)
        }
    }

    private fun unsigned32(value: ByteArray): ByteArray {
        val stripped = value.dropWhile { it == 0.toByte() }.toByteArray()
        require(stripped.size <= 32)
        return ByteArray(32 - stripped.size) + stripped
    }

    private fun encode(value: ByteArray): String = Base64.encodeToString(value, Base64.NO_WRAP)
    private fun decode(value: String): ByteArray = Base64.decode(value, Base64.NO_WRAP)

    companion object {
        private const val STRICT_ALIAS = "ProximityUnlock.Strict.v1"
        private const val RELAXED_ALIAS = "ProximityUnlock.Relaxed.v1"
        private const val ENCRYPTION_ALIAS = "ProximityUnlock.Secrets.v1"
        private const val MAX_SAFE_EVENTS = 16
    }
}
