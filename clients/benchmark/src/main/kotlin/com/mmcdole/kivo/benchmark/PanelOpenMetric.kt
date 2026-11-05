package com.mmcdole.kivo.benchmark

import androidx.benchmark.macro.ExperimentalMetricApi
import androidx.benchmark.macro.Metric.CaptureInfo
import androidx.benchmark.macro.Metric.Measurement
import androidx.benchmark.macro.TraceMetric
import androidx.benchmark.traceprocessor.TraceProcessor

/**
 * How long a panel takes to show after the OK that opens it, from the app's
 * last key event before the panel's first draw (the "Panel:draw" section),
 * OK's release: [OPEN] to the end of the frame that draws it, and [SHOWN] to
 * the end of the window animation that brings it in, where the system runs
 * one (the first long "animating" slice after the press).
 */
@OptIn(ExperimentalMetricApi::class)
class PanelOpenMetric : TraceMetric() {
    override fun getMeasurements(captureInfo: CaptureInfo, traceSession: TraceProcessor.Session): List<Measurement> {
        val app = captureInfo.targetPackageName
        val row = traceSession.query(
            """
            with app_slices as (
              select s.ts, s.dur, s.name, t.name as thread from slice s
              join thread_track tt on s.track_id = tt.id join thread t using (utid)
              join process p using (upid) where p.name = '$app'
            ),
            drawn as (select min(ts) as ts from app_slices where name = 'Panel:draw'),
            press as (select max(ts) as ts from app_slices where name like 'deliverInputEvent%' and ts < (select ts from drawn)),
            frame as (
              select min(ts + dur) as end_ts from app_slices
              where thread = 'RenderThread' and name like 'DrawFrames%' and ts >= (select ts from drawn)
            ),
            animation as (
              select s.ts + s.dur as end_ts from slice s
              where s.name = 'animating' and s.dur > 50e6 and s.ts > (select ts from press) and s.ts < (select ts from drawn) + 500e6
              order by s.ts limit 1
            )
            select (select end_ts from frame) - (select ts from press) as open_ns,
                   max((select end_ts from frame), coalesce((select end_ts from animation), 0)) - (select ts from press) as shown_ns
            """.trimIndent(),
        ).firstOrNull() ?: return emptyList()
        val open = row.nullableLong("open_ns") ?: return emptyList()
        val shown = row.nullableLong("shown_ns") ?: open
        return listOf(Measurement(OPEN, open / 1e6), Measurement(SHOWN, shown / 1e6))
    }

    private companion object {
        const val OPEN = "panelOpenMs"
        const val SHOWN = "panelShownMs"
    }
}
