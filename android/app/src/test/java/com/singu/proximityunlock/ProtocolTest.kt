package com.singu.proximityunlock

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec

class ProtocolTest {
    @Test
    fun fragmentsReassembleOutOfOrder() {
        val payload = ByteArray(421) { ((it * 31) and 0xff).toByte() }
        val fragments = Protocol.fragment(Protocol.MESSAGE_CHALLENGE, payload, 80)
        val reassembler = Protocol.Reassembler()
        var result: Pair<Byte, ByteArray>? = null
        for (fragment in fragments.reversed()) result = reassembler.add(fragment) ?: result
        assertNotNull(result)
        assertEquals(Protocol.MESSAGE_CHALLENGE, result!!.first)
        assertArrayEquals(payload, result!!.second)
    }

    @Test
    fun p256RawSignatureRoundTrips() {
        val keyPair = KeyPairGenerator.getInstance("EC").run {
            initialize(ECGenParameterSpec("secp256r1"))
            generateKeyPair()
        }
        val message = "ProximityUnlock Android protocol test".toByteArray()
        val der = Signature.getInstance("SHA256withECDSA").run {
            initSign(keyPair.private)
            update(message)
            sign()
        }
        val raw = Protocol.derToRaw(der)
        assertEquals(64, raw.size)
        val valid = Signature.getInstance("SHA256withECDSA").run {
            initVerify(keyPair.public)
            update(message)
            verify(Protocol.rawToDer(raw))
        }
        assertTrue(valid)
    }

    @Test
    fun hkdfMatchesRfc5869Vector() {
        val ikm = ByteArray(22) { 0x0b }
        val salt = ByteArray(13) { it.toByte() }
        val info = ByteArray(10) { (0xf0 + it).toByte() }
        val expected = "3cb25f25faacd57a90434f64d0362f2a" +
            "2d2d0a90cf1a5a4c5db02d56ecc4c5bf" +
            "34007208d5b887185865"
        assertEquals(expected, Protocol.hkdf(ikm, salt, info, 42).toHex())
    }

	@Test
	fun authenticationResponseMatchesWindowsWireSize() {
		val challenge = Protocol.Challenge(
			mode = Protocol.MODE_STRICT,
			issuedMs = 1,
			expiresMs = 5_001,
			pcId = ByteArray(16) { 1 },
			phoneId = ByteArray(16) { 2 },
			sessionId = 1,
			sidHash = ByteArray(32) { 3 },
			nonce = ByteArray(32) { 4 },
			signingBytes = ByteArray(122) { 5 },
			signature = ByteArray(64) { 6 },
		)
		val response = Protocol.response(challenge, 7) { ByteArray(64) { 8 } }
		assertEquals(182, response.size)
	}

    private fun ByteArray.toHex(): String = joinToString("") { "%02x".format(it) }
}
