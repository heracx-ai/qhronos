## Schedule Frequency Options

The `frequency` field in a schedule supports the following values:

- `minutely`: Every N minutes
- `hourly`: Every N hours
- `daily`: Every N days
- `weekly`: Every N weeks
- `monthly`: Every N months
- `yearly`: Every N years

Example schedule JSON:

```json
{
  "frequency": "hourly",
  "interval": 2
}
```

This will schedule the event every 2 hours. 