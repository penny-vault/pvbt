// Package tradecron extends standard cron syntax with awareness of trading
// calendars. It knows about market holidays, trading hours, and special
// sessions. The backtesting engine uses tradecron schedules to determine when
// to call a strategy's Compute method.
//
// # Schedule Syntax
//
// New returns a (*TradeCron, error). The first argument is a cron expression
// that may include market-aware directives. The second argument specifies
// trading session hours (typically RegularHours).
//
// Directives can appear before the standard cron fields:
//
//   - @open -- market open
//   - @close -- market close
//   - @weekbegin -- first trading day of the week
//   - @weekend -- last trading day of the week
//   - @monthbegin -- first trading day of the month
//   - @monthend -- last trading day of the month
//
// Directives may be combined with standard cron fields (minute, hour,
// day-of-month, month, day-of-week).
//
// # Common Schedules
//
//	@monthend             last trading day of each month
//	@close @monthend      last trading day at close
//	@close * * *          every trading day at close
//	@open * * *           every trading day at open
//	@monthbegin           first trading day of month
//	@weekbegin            first trading day of week
//	@weekend              last trading day of week
//	*/5 * * * *           every 5 minutes during trading hours
//
// # RegularHours
//
// RegularHours defines the standard US equity market session. When passed to
// New, it ensures the schedule never fires on weekends, holidays, or outside
// market hours. If a scheduled time falls on a holiday the schedule advances
// to the next valid trading day.
//
// # Dynamic Schedules
//
// A schedule is typically set during a strategy's Setup phase, but it can be
// modified during computation. This is useful for strategies that change
// behavior based on market conditions or other signals.
package tradecron
