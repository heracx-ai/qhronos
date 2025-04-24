# Qhronos API Documentation

## Authentication

All endpoints require authentication via a master token or JWT in the `Authorization` header:

```
Authorization: Bearer <token>
```

---

## Events

### Create Event
- **POST** `/events`
- **Body (One-Time Event):**
```json
{
  "name": "My Event",
  "description": "Description...",
  "start_time": "2024-04-23T10:00:00Z",
  "webhook": "https://example.com/webhook",
  "metadata": {"key": "value"},
  "tags": ["tag1", "tag2"]
}
```
- **Body (Recurring Event):**
```json
{
  "name": "My Recurring Event",
  "description": "Description...",
  "start_time": "2024-04-23T10:00:00Z",
  "webhook": "https://example.com/webhook",
  "schedule": {"frequency": "daily", "interval": 1},
  "metadata": {"key": "value"},
  "tags": ["tag1", "tag2"]
}
```
- **Note:**
  - For one-time events, omit the `schedule` field and provide `start_time`.
  - For recurring events, provide both `start_time` and a `schedule` object.
  - The `webhook` field is required (not `webhook_url`).
  - The `Authorization` header is required for all requests.
- **Response:** `201 Created`

### Get Event
- **GET** `/events/{id}`
- **Response:**
```json
{
  "id": "...",
  "name": "...",
  "description": "...",
  ...
}
```

### List Events
- **GET** `/events?tags=tag1,tag2`
- **Response:** Array of events

### Update Event
- **PUT** `/events/{id}`
- **Body:** Same as create
- **Response:** Updated event

### Delete Event
- **DELETE** `/events/{id}`
- **Response:** `204 No Content`

---

## Occurrences

### List Occurrences by Tags
- **GET** `/occurrences?tags=tag1,tag2`
- **Response:** Paginated list of occurrences

### List Occurrences by Event
- **GET** `/events/{id}/occurrences`
- **Response:** List of occurrences for the event

### Get Occurrence
- **GET** `/occurrences/{id}`
- **Response:** Occurrence details

---

## Tokens

### Create Token
- **POST** `/tokens`
- **Body:**
```json
{
  "type": "jwt",
  "sub": "user1",
  "access": "read",
  "scope": ["tag1", "tag2"],
  "expires_at": "2024-05-01T00:00:00Z"
}
```
- **Response:**
```json
{
  "token": "...",
  "type": "jwt",
  "sub": "user1",
  "access": "read",
  "scope": ["tag1", "tag2"],
  "expires_at": "2024-05-01T00:00:00Z"
}
```

---

## Error Responses

- Standard error format:
```json
{
  "error": "Error message"
}
```

---

For more details, see the [README](../README.md). 