package com.zarz.spotiflac

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.assertNull
import org.junit.Test

class SafMetadataReadPolicyTest {
    @Test fun completeMetadataReadErrorUsesTemporaryCopy() {
        val metadata = mapOf("lyrics" to "words", "comment" to "notes", "track_number" to 3)
        assertEquals(metadata, readSafMetadataWithFallback(
            directRead = { throw Exception("failed to read metadata: permission denied") },
            fallbackRead = { metadata },
        ))
    }

    @Test fun fallbackFailureIsIsolatedToOneFile() {
        assertNull(readSafMetadataWithFallback<String>({ null }, { throw IllegalArgumentException("malformed metadata") }))
        assertEquals("next file", readSafMetadataWithFallback({ "next file" }, { error("copy") }))
    }

    @Test fun seekableReadAvoidsTempCopy() {
        val metadata = mapOf("lyrics" to "words", "replaygain_track_gain" to "-6 dB")
        assertEquals(metadata, readSafMetadataWithFallback({ metadata }, { error("Unexpected copy") }))
    }

    @Test fun pipeAndRevokedUriDoNotDisableNextDescriptor() {
        var directReads = 0
        for (fails in listOf(true, false)) {
            var closed = false
            val value = readSafMetadataWithFallback(
                directRead = {
                    directReads++
                    try {
                        if (fails) throw SecurityException("revoked URI")
                        "descriptor metadata"
                    } finally { closed = true }
                },
                fallbackRead = {
                    assertTrue(closed)
                    "temporary metadata"
                },
            )
            assertEquals(if (fails) "temporary metadata" else "descriptor metadata", value)
        }
        assertEquals(2, directReads)
        assertEquals("pipe fallback", readSafMetadataWithFallback({ null }, { "pipe fallback" }))
        assertEquals("next provider", readSafMetadataWithFallback({ "next provider" }, { error("copy") }))
    }
}
