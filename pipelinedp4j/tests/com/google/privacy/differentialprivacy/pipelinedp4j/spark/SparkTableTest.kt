package com.google.privacy.differentialprivacy.pipelinedp4j.spark

import com.google.common.truth.Truth.assertThat
import com.google.privacy.differentialprivacy.pipelinedp4j.local.LocalCollection
import com.google.testing.junit.testparameterinjector.TestParameter
import com.google.testing.junit.testparameterinjector.TestParameterInjector
import org.apache.spark.sql.Encoders
import org.apache.spark.sql.SparkSession
import org.junit.AfterClass
import org.junit.BeforeClass
import org.junit.Test
import org.junit.runner.RunWith
import scala.Tuple2

@RunWith(TestParameterInjector::class)
class SparkTableTest {
    @Test
    fun keysEncoder_returnsCorrectEncoder() {
        val dataset = spark.createDataset(listOf(), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val result = sparkTable.keysEncoder

        assertThat(result).isInstanceOf(SparkEncoder::class.java)
        assertThat(result.encoder).isEqualTo(Encoders.STRING())
    }

    @Test
    fun valuesEncoder_returnsCorrectEncoder() {
        val dataset = spark.createDataset(listOf(), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val result = sparkTable.valuesEncoder

        assertThat(result).isInstanceOf(SparkEncoder::class.java)
        assertThat(result.encoder).isEqualTo(Encoders.INT())
    }

    @Test
    fun map_appliesMapFn() {
        val dataset = spark.createDataset(listOf(Tuple2(1, 10)), Encoders.tuple(Encoders.INT(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.INT(), Encoders.INT())
        val mapFn: (Int, Int) -> String = { k, v -> "${k}_$v" }
        val result = sparkTable.map("Test", sparkEncoderFactory.strings(), mapFn)
        assertThat(result.data.collectAsList()).containsExactly("1_10")
    }

    @Test
    fun groupAndCombineValues_appliesCombiner() {
        val dataset = spark.createDataset(listOf(Tuple2("positive", 1),
            Tuple2("positive", 10), Tuple2("negative", -1),
            Tuple2("negative", -10)
        ), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val combineFn: (Int, Int) -> Int = { v1, v2 -> v1 + v2 }
        val result = sparkTable.groupAndCombineValues("Test", combineFn)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2("positive", 11), Tuple2("negative", -11))
    }

    @Test
    fun groupByKey_groupsValues() {

    }

    @Test
    fun keys_returnKeys() {
        val data = spark.createDataset(listOf(Tuple2("key", "value")), Encoders.tuple(Encoders.STRING(), Encoders.STRING()))
        val sparkTable = SparkTable(data, Encoders.STRING(), Encoders.STRING())
        val result = sparkTable.keys("stageName")
        assertThat(result.data.collectAsList()).containsExactly("key")
    }

    @Test
    fun keys_returnsValues() {
        val data = spark.createDataset(listOf(Tuple2("key", "value")), Encoders.tuple(Encoders.STRING(), Encoders.STRING()))
        val sparkTable = SparkTable(data, Encoders.STRING(), Encoders.STRING())
        val result = sparkTable.values("stageName")
        assertThat(result.data.collectAsList()).containsExactly("value")
    }

    @Test
    fun mapValues_appliesMapFn() {
        val dataset = spark.createDataset(listOf(Tuple2("one", 1)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val mapFn: (String, Int) -> String = { k, v -> "${k}_$v" }

        val result = sparkTable.mapValues("stageName", sparkEncoderFactory.strings(), mapFn)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2("one", "one_1"))
    }

    @Test
    fun mapToTable_appliesMapFn() {
        val dataset = spark.createDataset(listOf(Tuple2("one", 1)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val mapFn: (String, Int) -> Pair<Int, String> = { k, v -> Pair(v, k) }
        val result = sparkTable.mapToTable("Test", sparkEncoderFactory.ints(), sparkEncoderFactory.strings(), mapFn)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2(1, "one"))
    }

    @Test
    fun flatMapToTable_appliesMapFn() {
        val dataset = spark.createDataset(listOf(Tuple2("one", 1)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val mapFn: (String, Int) -> Sequence<Pair<Int, String>> = { k, v ->
            sequenceOf(Pair(v, k), Pair(v, k))
        }
        val result = sparkTable.flatMapToTable("Test", sparkEncoderFactory.ints(), sparkEncoderFactory.strings(), mapFn)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2(1,"one"), Tuple2(1, "one"))
    }

    @Test
    fun filterValues_appliesPredicate() {
        val dataset = spark.createDataset(listOf(Tuple2("one", 1), Tuple2("two", 2)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val predicate: (Int) -> Boolean = { v -> v == 1 }
        val result = sparkTable.filterValues("Test", predicate)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2("one", 1))
    }

    @Test
    fun filterKeys_allowedKeysStoredInBeamCollection_keepsOnlyAllowedKeys(@TestParameter unbalancedKeys: Boolean) {
        val dataset = spark.createDataset(listOf(Tuple2("one", 1), Tuple2("two", 2), Tuple2("three", 3),
            Tuple2("two", -2)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val allowedKeysCollection = spark.createDataset(listOf("three", "two", "four"), Encoders.STRING())
        val allowedKeysDataset = SparkCollection(allowedKeysCollection)
        val result = sparkTable.filterKeys("stageName", allowedKeysDataset, unbalancedKeys)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2("two", 2), Tuple2("three", 3),
            Tuple2("two", -2))
    }

    @Test
    fun filterKeys_allowedKeysStoredInLocalCollection_keepsOnlyAllowedKeys() {
        val dataset = spark.createDataset(listOf(Tuple2("one", 1), Tuple2("two", 2), Tuple2("three", 3),
            Tuple2("two", -2)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val allowedKeys = sequenceOf("three", "two", "four")
        val allowedKeysLocalCollection = LocalCollection(allowedKeys)
        val result = sparkTable.filterKeys("stageName", allowedKeysLocalCollection)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2("two", 2), Tuple2("three", 3),
            Tuple2("two", -2))
    }

    @Test
    fun flattenWith_flattensCollections() {
        val dataset = spark.createDataset(listOf(Tuple2("one", 1)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val sparkTable = SparkTable(dataset, Encoders.STRING(), Encoders.INT())
        val otherSparkDataset = spark.createDataset(listOf(Tuple2("two", 2)), Encoders.tuple(Encoders.STRING(), Encoders.INT()))
        val otherSparkTable = SparkTable(otherSparkDataset, Encoders.STRING(), Encoders.INT())

        val result = sparkTable.flattenWith("stageName", otherSparkTable)
        assertThat(result.data.collectAsList()).containsExactly(Tuple2("one", 1), Tuple2("two", 2))
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
                    .config("spark.driver.bindAddress", "127.0.0.1")
                    .getOrCreate();
            } catch (e: Exception) {
                e.printStackTrace()
            }
        }
        @AfterClass
        @JvmStatic
        fun tearDown() {
            // Stop SparkSession after all tests are done
            if (::spark.isInitialized) {
                spark.stop()
            }
        }
    }
}