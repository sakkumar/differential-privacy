/*
 * Copyright 2024 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package com.google.privacy.differentialprivacy.pipelinedp4j.spark

import org.apache.spark.sql.SparkSession
import org.junit.runner.RunWith
import org.junit.runners.Suite

/** Provides a list of JUnit test classes to Bazel. When creating a new test class, add it here. */
@RunWith(Suite::class)
@Suite.SuiteClasses(SparkCollectionTest::class, SparkEncodersTest::class, SparkTableTest::class)
class SparkTests {}

// Helper method to create SparkSession
fun createSparkSession(): SparkSession {
    return SparkSession.builder()
        .appName("Kotlin Spark Example")
        .master("local[*]")
        .config("spark.driver.bindAddress", "127.0.0.1")
        .getOrCreate()
}

// Helper method to stop SparkSession
fun stopSparkSession(spark: SparkSession) {
    spark.stop()
}
