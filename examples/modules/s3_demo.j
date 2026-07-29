#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Put, get, list, and delete an object in an S3-compatible bucket, signing every
 * request with AWS Signature Version 4. Needs real credentials, so it reads them
 * from the environment and prints usage when they are absent (works with AWS S3,
 * MinIO, Cloudflare R2, Backblaze B2 - just point S3_ENDPOINT at the store).
 * @module s3_demo
 */
use io;
use os;
import "../../modules/s3.j" as s3;
import "../../modules/http.j" as http;

# Presigned URLs are pure signing (no network), so this part always runs and
# needs no credentials - it shows the SigV4 query-signed link format.
def demo as s3.Client init s3.connect(
    "https://s3.us-east-1.amazonaws.com",
    "us-east-1",
    "AKIAIOSFODNN7EXAMPLE",
    "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY");
io.printf("presigned GET (1 h): %s\n\n", s3.presign($demo, "GET", "mybucket", "report.pdf", 3600));

def endpoint as string init os.getEnv("S3_ENDPOINT");
def region as string init os.getEnv("S3_REGION");
def key as string init os.getEnv("S3_KEY");
def secret as string init os.getEnv("S3_SECRET");
def store as string init os.getEnv("S3_BUCKET");

if ($endpoint == "" or $key == "" or $store == "") {
    io.printf("Set S3_ENDPOINT / S3_REGION / S3_KEY / S3_SECRET / S3_BUCKET to run a live demo.\n");
    io.printf("  MinIO example:  S3_ENDPOINT=http://localhost:9000 S3_REGION=us-east-1 \\\n");
    io.printf("                  S3_KEY=minioadmin S3_SECRET=minioadmin S3_BUCKET=test\n");
    exit;
}

def client as s3.Client init s3.connect($endpoint, $region, $key, $secret);

def putRes as http.Response init s3.put(
    $client,
    $store,
    "jennifer-demo.txt",
    "hello from jennifer");
io.printf("put    jennifer-demo.txt -> %d\n", $putRes.status);

def getRes as http.Response init s3.get($client, $store, "jennifer-demo.txt");
io.printf("get    jennifer-demo.txt -> %d  body=%s\n", $getRes.status, $getRes.body);

def listRes as http.Response init s3.listObjects($client, $store);
io.printf("list   %s -> %d\n", $store, $listRes.status);
for (def k in s3.objectKeys($listRes.body)) {
    io.printf("  - %s\n", $k);
}

def delRes as http.Response init s3.delete($client, $store, "jennifer-demo.txt");
io.printf("delete jennifer-demo.txt -> %d\n", $delRes.status);
