# API Traffic Report

A command-line program that reads a JSONL API request log and prints a single JSON traffic report to stdout.

## How to run

Requires **Go 1.23+** (tested with `go1.23.4 darwin/arm64`).

```bash
go build -o apitrafficreport .

./apitrafficreport sample_input/requests.jsonl
```

It also reads from stdin if no path is given (or `-` is passed):

```bash
cat sample_input/requests.jsonl | ./apitrafficreport
```

Optional flags to tune the rate-limit rule:

```bash
./apitrafficreport -limit 100 -window 1m sample_input/requests.jsonl
```

Run `./apitrafficreport -h` for full flag documentation.

## Solution design

### End to end

1. Read the input line by line (file arg, or stdin) using a buffered scanner.
2. Parse and validate each line as JSON with the required fields (`request_id`, `timestamp`, `client_id`, `endpoint`, `status_code`). Any parse/validation failure increments a `malformed_count` and the line is discarded; the program never crashes on bad input.
3. Group valid records by `client_id`, sort each client's requests by timestamp.
4. For each client, run a sliding-window scan to detect rate-limit violations and compute the busiest window size.
5. Aggregate overall status-code buckets (2xx/3xx/4xx/5xx/other) and per-client endpoint counts.
6. Emit one JSON object to stdout containing a summary, the rate-limit rule used, status code counts, and a per-client breakdown.

### Rate-limit rule chosen (and why)

**Sliding window: more than N requests within any T-second window is a violation.** Defaults: `N=5`, `T=10s`, configurable via `-limit` / `-window`.

A sliding window (rather than fixed buckets, e.g. "per calendar minute") was chosen because it doesn't let clients burst across a bucket boundary (e.g. 5 requests at :00:59 and 5 more at :01:00 would look fine in fixed buckets but is a real 10-req burst). It's also the closest analogue to how most real API gateways (e.g. token bucket / leaky bucket derivatives) reason about abuse, and it's computable in O(n) per client with a two-pointer scan over sorted timestamps — no need to know the limit ahead of time or pre-bucket data.

I picked `5 requests / 10s` as a generic, conservative default for a "reasonable" limit given no product-specific SLA was provided. It's intentionally exposed as a flag since the "right" number is a product/business decision I don't have visibility into, and different endpoints/clients may reasonably need different limits in a real system (see "what I'd do differently").

The report also outputs `max_requests_in_window` per client (the busiest N-second stretch), for visibility even when a client is inside the limit.

### Output specification

The program prints one JSON object to stdout:

```jsonc
{
  "generated_at": "<RFC3339 timestamp, when the report was generated>",
  "summary": {
    "total_lines": 8,          // total non-blank lines read
    "valid_count": 8,          // lines that parsed and validated successfully
    "malformed_count": 0       // lines discarded as malformed
  },
  "malformed_samples": ["..."],// up to 10 example reasons for discarded lines (omitted if none)
  "rate_limit_rule": {
    "max_requests": 5,
    "window_seconds": 10,
    "description": "A client violates the rate limit if it makes more than 5 requests within any sliding 10s window."
  },
  "status_code_counts": { "2xx": 8 },   // overall counts bucketed by status class
  "clients": [
    {
      "client_id": "acct_1",
      "request_count": 6,
      "first_seen": "2024-01-15T10:00:00Z",
      "last_seen": "2024-01-15T10:00:08Z",
      "endpoints": { "/v1/widgets": 6 },     // per-endpoint request counts for this client
      "rate_limit_violations": 1,            // number of requests that pushed the window over the limit
      "max_requests_in_window": 6            // busiest N-second window observed for this client
    }
  ]
}
```

`clients` is sorted by `client_id` ascending for deterministic, diffable output.

### Implementation decisions and assumptions

- **A line is "malformed" if:** it isn't valid JSON, or any of `request_id`/`client_id`/`endpoint`/`timestamp` is missing/empty, or `timestamp` doesn't parse as RFC3339 (covers both `Z` and `+HH:MM` offsets, per the spec), or `status_code` isn't an integer in the `100–599` range. Blank lines are silently skipped (not counted as malformed — treated as harmless formatting, not a data record).
- Malformed lines are **discarded, not fatal** — the program processes the rest of the file and reports a count (plus up to 10 sample reasons for debuggability) rather than exiting non-zero, since this runs as an unattended observability service and one bad line from an upstream shouldn't kill the report.
- Timestamps are normalized to UTC for display/comparison, so offset (`+HH:MM`) and `Z` timestamps compare correctly against each other.
- `request_id` isn't deduplicated against — the spec doesn't say IDs are guaranteed unique in practice, and de-duping wasn't a stated requirement, so I left it as a straightforward count-based report. Flagging duplicate `request_id`s would be a good enhancement (see below).
- Rate-limit violation count is **request-count based** (each request that exceeds the window threshold counts once), rather than counting distinct offending windows, since it maps most directly to "how many requests should have been rejected."

### What I'd do differently with more time

- Make rate limits configurable **per endpoint** (e.g. write-heavy endpoints usually need tighter limits than read-only ones), not just globally per client.
- Emit malformed-line **line numbers** in addition to sample reasons, to make upstream debugging easier.
- Support streaming very large files without holding all records in memory (currently records are grouped by client in memory,.
- And add unit tests covering more malformed-input edge cases (timezone edge cases, huge files, out-of-order timestamps).

### AI tool usage

Used ChatGPT and Gemini to write small chunks of code, add code comments, test data and draft the `README.md` from my initial brain dump. I also incorporated suggestions and improvements from both tools.
Also had one whole main(pretty ugly) with everthing in, took help from these LLMs to split them in seperate go files.

