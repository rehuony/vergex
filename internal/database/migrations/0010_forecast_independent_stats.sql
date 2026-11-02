ALTER TABLE forecast.outcome_stats
    ADD COLUMN independent_misses   bigint NOT NULL DEFAULT 0,
    ADD COLUMN independent_move_sum double precision NOT NULL DEFAULT 0;
COMMENT ON COLUMN forecast.outcome_stats.independent_misses IS 'Independent calls that were wrong. With independent_hits, an honest hit rate.';
COMMENT ON COLUMN forecast.outcome_stats.independent_move_sum IS 'Sum of actual_move_pct over independent signals.';

UPDATE forecast.outcome_stats AS o
SET independent_misses = s.misses, independent_move_sum = s.move_sum
FROM (
    SELECT strategy_id, series_id, date_trunc('month', open_at AT TIME ZONE 'UTC')::date AS month,
           direction, regime, LEAST(confidence / 20, 4) AS confidence_band,
           COUNT(*) FILTER (WHERE outcome = 'miss') AS misses, SUM(actual_move_pct) AS move_sum
    FROM forecast.signals
    WHERE independent
    GROUP BY 1, 2, 3, 4, 5, 6
) AS s
WHERE o.strategy_id = s.strategy_id AND o.series_id = s.series_id AND o.month = s.month
  AND o.direction = s.direction AND o.regime = s.regime AND o.confidence_band = s.confidence_band;
