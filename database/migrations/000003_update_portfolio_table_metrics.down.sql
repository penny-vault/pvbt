--- Remove metrics to portfolio table
BEGIN;

ALTER TABLE portfolio DROP COLUMN cagr_3yr;
ALTER TABLE portfolio DROP COLUMN cagr_5yr;
ALTER TABLE portfolio DROP COLUMN cagr_10yr;
ALTER TABLE portfolio DROP COLUMN std_dev;
ALTER TABLE portfolio DROP COLUMN downside_deviation;
ALTER TABLE portfolio DROP COLUMN max_draw_down;
ALTER TABLE portfolio DROP COLUMN avg_draw_down;
ALTER TABLE portfolio DROP COLUMN sharpe_ratio;
ALTER TABLE portfolio DROP COLUMN sortino_ratio;
ALTER TABLE portfolio DROP COLUMN ulcer_index;

COMMIT;