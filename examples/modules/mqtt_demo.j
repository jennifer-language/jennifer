#!/usr/bin/env -S jennifer run
# SPDX-License-Identifier: LGPL-3.0-only
# Copyright (C) 2026 mplx <jennifer@mplx.dev>

/**
 * Publish and subscribe over MQTT with the mqtt module.
 * Needs an MQTT broker listening on 127.0.0.1:1883 (e.g. mosquitto) and the default `jennifer` binary (the module uses `net`). With no broker running it prints the connection error rather than failing. Not a golden test (it needs a live broker); it demonstrates the surface.
 * @module mqtt_demo
 */
use io;
use convert;
import "../../modules/mqtt.j" as mqtt;

def opts as mqtt.Options init mqtt.Options{host: "127.0.0.1", port: 1883,
    clientId: "jennifer-demo", keepalive: 30, security: "none",
    username: "", password: ""};

try {
    def c as mqtt.Client init mqtt.connect($opts);
    io.printf("connected\n");

    mqtt.subscribe($c, "jennifer/demo");
    io.printf("subscribed to jennifer/demo\n");

    # Publish a few messages; the broker routes each back to our subscription.
    def i as int init 0;
    while ($i < 3) {
        mqtt.publish($c, "jennifer/demo", "message " + convert.toString($i));
        $i = $i + 1;
    }

    # Drain what came back, waiting up to 1s per message.
    def seen as int init 0;
    def draining as bool init true;
    while ($draining) {
        def msgs as list of mqtt.Message init mqtt.poll($c, 1000);
        if (len($msgs) > 0) {
            def m as mqtt.Message init $msgs[0];
            io.printf("  %s -> %s\n", $m.topic,
                convert.stringFromBytes($m.payload, "utf-8"));
            $seen = $seen + 1;
            if ($seen == 3) {
                $draining = false;
            }
        } else {
            # Nothing pending within the window - stop draining.
            $draining = false;
        }
    }

    mqtt.disconnect($c);
    io.printf("disconnected\n");
} catch (err) {
    io.printf("mqtt demo needs a broker on 127.0.0.1:1883: %s\n", $err.message);
}
