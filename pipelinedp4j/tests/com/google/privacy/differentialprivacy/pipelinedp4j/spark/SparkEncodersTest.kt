package com.google.privacy.differentialprivacy.pipelinedp4j.spark

import com.google.common.truth.Truth.assertThat
import com.google.privacy.differentialprivacy.pipelinedp4j.core.ContributionWithPrivacyId
import com.google.privacy.differentialprivacy.pipelinedp4j.core.contributionWithPrivacyId
import com.google.privacy.differentialprivacy.pipelinedp4j.core.encoderOfContributionWithPrivacyId
import com.google.privacy.differentialprivacy.pipelinedp4j.proto.CompoundAccumulator
import com.google.privacy.differentialprivacy.pipelinedp4j.proto.compoundAccumulator
import com.google.privacy.differentialprivacy.pipelinedp4j.proto.meanAccumulator
import com.google.privacy.differentialprivacy.pipelinedp4j.proto.quantilesAccumulator
import com.google.privacy.differentialprivacy.pipelinedp4j.proto.sumAccumulator
import com.google.protobuf.ByteString
import org.apache.spark.sql.SparkSession
import org.junit.AfterClass
import org.junit.BeforeClass
import org.junit.Test
import org.junit.runner.RunWith
import org.junit.runners.JUnit4

@RunWith(JUnit4::class)
class SparkEncodersTest {


    @Test
    fun strings_isPossibleToCreateSparkCollectionOfThatType() {
        val input = listOf("a", "b", "c")
        val inputCoder = sparkEncoderFactory.strings().encoder
        val dataset = spark.createDataset(input, inputCoder)
        assertThat(dataset.collectAsList()).containsExactlyElementsIn(input)
    }

    @Test
    fun doubles_isPossibleToCreateSparkCollectionOfThatType() {
        val input = listOf(-1.2, 0.0, 2.1)
        val inputCoder = sparkEncoderFactory.doubles().encoder
        val dataset = spark.createDataset(input, inputCoder)
        assertThat(dataset.collectAsList()).containsExactlyElementsIn(input)
    }

    @Test
    fun ints_isPossibleToCreateSparkCollectionOfThatType() {
        val input = listOf(-1, 0, 1)
        val inputCoder = sparkEncoderFactory.ints().encoder
        val dataset = spark.createDataset(input, inputCoder)
        assertThat(dataset.collectAsList()).containsExactlyElementsIn(input)
    }

    @Test
    fun records_isPossibleToCreateBeamPCollectionOfThatType() {
//        val input =
//            listOf(
//                contributionWithPrivacyId("privacyId1", "partitionKey1", -1.0),
//                contributionWithPrivacyId("privacyId2", "partitionKey1", 0.0),
//                contributionWithPrivacyId("privacyId1", "partitionKey2", 1.0),
//                contributionWithPrivacyId("privacyId3", "partitionKey3", 1.2345),
//            )
//        val inputCoder =
//            (encoderOfContributionWithPrivacyId(
//                sparkEncoderFactory.strings(),
//                sparkEncoderFactory.strings(),
//                sparkEncoderFactory,
//            ) as SparkEncoder<ContributionWithPrivacyId<String, String>>).encoder
//
//        val dataset = spark.createDataset(input, inputCoder)
//        assertThat(dataset.collectAsList()).containsExactlyElementsIn(input)
    }

    @Test
    fun protos_isPossibleToCreateBeamPCollectionOfThatType() {
        val input =
            listOf(
                compoundAccumulator {
                    sumAccumulator = sumAccumulator { sum = -123.0 }
                    meanAccumulator = meanAccumulator {
                        count = 12
                        normalizedSum = -1.543
                    }
                    quantilesAccumulator = quantilesAccumulator {
                        serializedQuantilesSummary =
                            ByteString.copyFrom(byteArrayOf(0x48, 0x65, 0x6c, 0x6c, 0x6f))
                    }
                },
                compoundAccumulator {},
            )
        val inputCoder = sparkEncoderFactory.protos(CompoundAccumulator::class).encoder
        val dataset = spark.createDataset(input, inputCoder)
        assertThat(dataset.collectAsList()).containsExactlyElementsIn(input)
    }

    @Test
    fun tuple2sOf_isPossibleToCreateSparkCollectionOfThatType() {

    }

    companion object {
        private val sparkEncoderFactory = SparkEncoderFactory()
        private lateinit var spark: SparkSession
        @BeforeClass
        @JvmStatic
        fun setup() {
            try {
                spark = SparkSession.builder()
                    .appName("Kotlin Spark Example")
                    .master("local[*]")
                    .getOrCreate();
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }
        @AfterClass
        @JvmStatic
        fun tearDown() {
            // Stop SparkSession after all tests are done
            spark.stop()
        }
    }
}