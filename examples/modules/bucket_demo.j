#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Put, get, list, and delete an object in an S3-compatible bucket, signing every
 * request with AWS Signature Version 4. Needs real credentials, so it reads them
 * from the environment and prints usage when they are absent (works with AWS S3,
 * MinIO, Cloudflare R2, Backblaze B2 - just point S3_ENDPOINT at the store).
 * @module bucket_demo
 */
use io;
use os;
import "../../modules/bucket.j" as bucket;
import "../../modules/http.j" as http;

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

def client as bucket.Client init bucket.connect($endpoint, $region, $key, $secret);

def putRes as http.Response init bucket.put($client, $store, "jennifer-demo.txt", "hello from jennifer");
io.printf("put    jennifer-demo.txt -> %d\n", $putRes.status);

def getRes as http.Response init bucket.get($client, $store, "jennifer-demo.txt");
io.printf("get    jennifer-demo.txt -> %d  body=%s\n", $getRes.status, $getRes.body);

def listRes as http.Response init bucket.listObjects($client, $store);
io.printf("list   %s -> %d\n", $store, $listRes.status);
for (def k in bucket.objectKeys($listRes.body)) {
    io.printf("  - %s\n", $k);
}

def delRes as http.Response init bucket.delete($client, $store, "jennifer-demo.txt");
io.printf("delete jennifer-demo.txt -> %d\n", $delRes.status);
