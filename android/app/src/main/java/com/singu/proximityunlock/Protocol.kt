package com.singu.proximityunlock

import android.net.Uri
import android.util.Base64
import java.math.BigInteger
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.security.AlgorithmParameters
import java.security.KeyFactory
import java.security.MessageDigest
import java.security.PublicKey
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

object Protocol {
    const val VERSION: Byte = 1
    const val MODE_STRICT: Byte = 1
    const val MODE_CONVENIENCE: Byte = 2
    const val MESSAGE_CHALLENGE: Byte = 1
    const val MESSAGE_PAIRING: Byte = 2
	const val RESPONSE_BODY_SIZE = 118

    val SERVICE_UUID = java.util.UUID.fromString("9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a001")
    val CHALLENGE_UUID = java.util.UUID.fromString("9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a002")
    val RESPONSE_UUID = java.util.UUID.fromString("9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a003")
    val PAIRING_UUID = java.util.UUID.fromString("9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a004")
    val ADVERTISEMENT_UUID = java.util.UUID.fromString("0000fff0-0000-1000-8000-00805f9b34fb")
	const val ADVERTISEMENT_COMPANY_ID = 0xffff
    val CCC_UUID = java.util.UUID.fromString("00002902-0000-1000-8000-00805f9b34fb")

    private val AD_CONTEXT = "ProximityUnlock/ad/v1".toByteArray()
    private val PAIR_AD_CONTEXT = "ProximityUnlock/pair-ad/v1".toByteArray()
    private val PAIR_CONTEXT = "ProximityUnlock/pair/v1".toByteArray()

    data class PairingUri(
        val pcId: ByteArray,
        val pcPublicKey: ByteArray,
        val secret: ByteArray,
        val expiresAtSeconds: Long,
    )

    data class Challenge(
        val mode: Byte,
        val issuedMs: Long,
        val expiresMs: Long,
        val pcId: ByteArray,
        val phoneId: ByteArray,
        val sessionId: Int,
        val sidHash: ByteArray,
        val nonce: ByteArray,
        val signingBytes: ByteArray,
        val signature: ByteArray,
    )

    data class PairChallenge(
        val pcId: ByteArray,
        val nonce: ByteArray,
    )

    fun parsePairingUri(raw: String, nowSeconds: Long = System.currentTimeMillis() / 1000): PairingUri {
        val uri = Uri.parse(raw.trim())
        require(uri.scheme == "proximityunlock" && uri.host == "pair") { "Invalid pairing URI" }
        require(uri.getQueryParameter("v") == "1") { "Unsupported pairing version" }
        val pc = b64(uri.getQueryParameter("pc") ?: "")
        val pub = b64(uri.getQueryParameter("pub") ?: "")
        val secret = b64(uri.getQueryParameter("secret") ?: "")
        val expiry = uri.getQueryParameter("exp")?.toLongOrNull() ?: 0
        require(pc.size == 16 && pub.size == 65 && pub[0] == 4.toByte() && secret.size == 32) { "Invalid pairing fields" }
        require(expiry > nowSeconds && expiry <= nowSeconds + 125) { "Pairing code expired" }
        return PairingUri(pc, pub, secret, expiry)
    }

    fun presenceKey(secret: ByteArray, pcId: ByteArray, phoneId: ByteArray): ByteArray {
        val salt = sha256(PAIR_CONTEXT + pcId + phoneId)
        return hkdf(secret, salt, "presence-key".toByteArray(), 32)
    }

    fun rollingAdvertisement(presenceKey: ByteArray): ByteArray {
        val salt = ByteArray(4).also { java.security.SecureRandom().nextBytes(it) }
        val tag = hmac(presenceKey, AD_CONTEXT + salt)
        return byteArrayOf(VERSION) + salt + tag.copyOf(8)
    }

    fun pairingAdvertisement(secret: ByteArray, pcId: ByteArray): ByteArray {
        val tag = hmac(secret, PAIR_AD_CONTEXT + pcId).copyOf(8)
        return byteArrayOf((0x80 or VERSION.toInt()).toByte()) + pcId.copyOf(4) + tag
    }

    fun parsePairingChallenge(data: ByteArray, secret: ByteArray): PairChallenge {
        require(data.size == 85 && data.copyOfRange(0, 4).contentEquals("PUQ1".toByteArray()) && data[4] == VERSION)
        val body = data.copyOfRange(0, 53)
        val expected = hmac(secret, body)
        require(MessageDigest.isEqual(expected, data.copyOfRange(53, 85))) { "Pairing MAC failed" }
        return PairChallenge(data.copyOfRange(5, 21), data.copyOfRange(21, 53))
    }

    fun pairingResponse(
        challenge: PairChallenge,
        phoneId: ByteArray,
        strictPublic: ByteArray,
        relaxedPublic: ByteArray,
        secret: ByteArray,
    ): ByteArray {
        require(phoneId.size == 16 && strictPublic.size == 65 && relaxedPublic.size == 65)
        val body = "PUP1".toByteArray() + byteArrayOf(VERSION) + challenge.pcId + phoneId + strictPublic + relaxedPublic + challenge.nonce
        return body + hmac(secret, body)
    }

    fun parseChallenge(data: ByteArray, pcPublic: PublicKey, expectedPcId: ByteArray, expectedPhoneId: ByteArray): Challenge {
        require(data.size == 186 && data.copyOfRange(0, 4).contentEquals("PUC1".toByteArray())) { "Invalid challenge" }
        val body = data.copyOfRange(0, 122)
        val b = ByteBuffer.wrap(body).order(ByteOrder.BIG_ENDIAN)
        b.position(4)
        require(b.get() == VERSION)
        val mode = b.get()
        require(mode == MODE_STRICT || mode == MODE_CONVENIENCE)
        val issued = b.long
        val expires = b.long
        val pcId = ByteArray(16).also { b.get(it) }
        val phoneId = ByteArray(16).also { b.get(it) }
        val sessionId = b.int
        val sidHash = ByteArray(32).also { b.get(it) }
        val nonce = ByteArray(32).also { b.get(it) }
        val now = System.currentTimeMillis()
        require(expires > issued && expires - issued <= 5000 && now >= issued - 1000 && now <= expires) { "Expired challenge" }
        require(pcId.contentEquals(expectedPcId) && phoneId.contentEquals(expectedPhoneId)) { "Wrong device identity" }
        val signature = data.copyOfRange(122, 186)
        val verifier = Signature.getInstance("SHA256withECDSA")
        verifier.initVerify(pcPublic)
        verifier.update(body)
        require(verifier.verify(rawToDer(signature))) { "PC signature failed" }
        return Challenge(mode, issued, expires, pcId, phoneId, sessionId, sidHash, nonce, body, signature)
    }

    fun response(challenge: Challenge, counter: Long, signer: (ByteArray) -> ByteArray): ByteArray {
        val body = ByteBuffer.allocate(RESPONSE_BODY_SIZE).order(ByteOrder.BIG_ENDIAN).apply {
            put("PUR1".toByteArray())
            put(VERSION)
            put(challenge.mode)
            putLong(System.currentTimeMillis())
            putLong(counter)
            put(challenge.pcId)
            put(challenge.phoneId)
            put(challenge.nonce)
            put(sha256(challenge.signingBytes))
        }.array()
        return body + signer(body)
    }

    fun publicKey(sec1: ByteArray): PublicKey {
        require(sec1.size == 65 && sec1[0] == 4.toByte())
        val params = AlgorithmParameters.getInstance("EC").apply { init(ECGenParameterSpec("secp256r1")) }
            .getParameterSpec(ECParameterSpec::class.java)
        val point = ECPoint(BigInteger(1, sec1.copyOfRange(1, 33)), BigInteger(1, sec1.copyOfRange(33, 65)))
        return KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(point, params))
    }

    fun derToRaw(der: ByteArray): ByteArray {
        require(der.size >= 8 && der[0] == 0x30.toByte())
        var p = 2
        require(der[p++] == 0x02.toByte())
        val rLen = der[p++].toInt() and 0xff
        val r = der.copyOfRange(p, p + rLen)
        p += rLen
        require(der[p++] == 0x02.toByte())
        val sLen = der[p++].toInt() and 0xff
        val s = der.copyOfRange(p, p + sLen)
        return unsigned32(r) + unsigned32(s)
    }

    fun rawToDer(raw: ByteArray): ByteArray {
        require(raw.size == 64)
        fun integer(part: ByteArray): ByteArray {
            val withoutLeadingZeroes = part.dropWhile { it == 0.toByte() }.toByteArray()
            val stripped = if (withoutLeadingZeroes.isEmpty()) byteArrayOf(0) else withoutLeadingZeroes
            return if (stripped[0].toInt() and 0x80 != 0) byteArrayOf(0) + stripped else stripped
        }
        val r = integer(raw.copyOfRange(0, 32))
        val s = integer(raw.copyOfRange(32, 64))
        return byteArrayOf(0x30, (2 + r.size + 2 + s.size).toByte(), 0x02, r.size.toByte()) + r +
            byteArrayOf(0x02, s.size.toByte()) + s
    }

    private fun unsigned32(value: ByteArray): ByteArray {
        val stripped = value.dropWhile { it == 0.toByte() }.toByteArray()
        require(stripped.size <= 32)
        return ByteArray(32 - stripped.size) + stripped
    }

    fun fragment(type: Byte, payload: ByteArray, mtuPayload: Int = 180): List<ByteArray> {
        val partSize = mtuPayload - 3
        val count = maxOf(1, (payload.size + partSize - 1) / partSize)
        require(count <= 255)
        return (0 until count).map { index ->
            val from = index * partSize
            val to = minOf(payload.size, from + partSize)
            byteArrayOf(type, index.toByte(), count.toByte()) + payload.copyOfRange(from, to)
        }
    }

    class Reassembler {
        private var type: Byte? = null
        private var parts: Array<ByteArray?>? = null

        @Synchronized
        fun add(fragment: ByteArray): Pair<Byte, ByteArray>? {
            require(fragment.size >= 3)
            val fragmentType = fragment[0]
            val index = fragment[1].toInt() and 0xff
            val count = fragment[2].toInt() and 0xff
            require(count > 0 && index < count)
            if (parts == null) {
                type = fragmentType
                parts = arrayOfNulls(count)
            }
            require(type == fragmentType && parts!!.size == count)
            if (parts!![index] == null) parts!![index] = fragment.copyOfRange(3, fragment.size)
            if (parts!!.any { it == null }) return null
            val result = parts!!.filterNotNull().fold(ByteArray(0)) { acc, bytes -> acc + bytes }
            parts = null
            type = null
            return fragmentType to result
        }
    }

    fun hmac(key: ByteArray, data: ByteArray): ByteArray = Mac.getInstance("HmacSHA256").run {
        init(SecretKeySpec(key, "HmacSHA256"))
        doFinal(data)
    }

    fun hkdf(input: ByteArray, salt: ByteArray, info: ByteArray, length: Int): ByteArray {
        val prk = hmac(salt, input)
        var previous = ByteArray(0)
        var result = ByteArray(0)
        var counter = 1
        while (result.size < length) {
            previous = hmac(prk, previous + info + byteArrayOf(counter.toByte()))
            result += previous
            counter++
        }
        prk.fill(0)
        previous.fill(0)
        return result.copyOf(length)
    }

    fun sha256(data: ByteArray): ByteArray = MessageDigest.getInstance("SHA-256").digest(data)
    private fun b64(value: String): ByteArray = Base64.decode(value, Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)
    fun b64(data: ByteArray): String = Base64.encodeToString(data, Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP)
}
