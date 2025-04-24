# Qhronos TypeScript Client SDK – Design Proposal

## Overview
This document outlines the design for a TypeScript client SDK for Qhronos, intended for distribution via npm and usable in both Node.js and browser environments. The SDK provides a full-featured, type-safe, and developer-friendly interface to the Qhronos REST API.

---

## Function List

### Event Management
1. `createEvent(data: CreateEventRequest): Promise<Event>`
2. `getEvent(id: string): Promise<Event>`
3. `listEvents(params?: ListEventsParams): Promise<Event[]>`
4. `updateEvent(id: string, data: UpdateEventRequest): Promise<Event>`
5. `deleteEvent(id: string): Promise<void>`
6. `listOccurrences(eventId: string, params?: ListOccurrencesParams): Promise<Occurrence[]>`
7. `getOccurrence(occurrenceId: string): Promise<Occurrence>`

### Authentication & Tokens
8. `setAuthToken(token: string): void`
9. `logout(): void`  
   *(client-side only, clears token)*

### Admin/Meta
10. `getStatus(): Promise<SystemStatus>`

### Tag & Metadata Management
11. `getEventsByTag(tag: string): Promise<Event[]>`

### Helper/Utility Methods
12. `waitForEventStatus(id: string, status: string, timeoutMs?: number): Promise<Event>`

### Types/Interfaces
- All request/response types for the above methods will be defined for type safety and developer experience.

---

## Design Notes
- **Framework Agnostic:** Uses the Fetch API (polyfilled for Node.js if needed).
- **Auth Handling:** Supports JWT/master token, with methods to set/update tokens.
- **Type Safety:** All request/response payloads are strongly typed.
- **Helper Methods:** For common workflows (e.g., polling for event status).
- **Distribution:** To be published as `qhronos-client` on npm, supporting both ESM and CJS.

---

## Example Usage
```typescript
import { QhronosClient } from 'qhronos-client';

const qhronos = new QhronosClient({
  baseUrl: 'https://api.qhronos.example.com',
  authToken: 'your-jwt-or-master-token',
});

const event = await qhronos.createEvent({
  name: 'Daily Report',
  start_time: '2024-06-01T09:00:00Z',
  webhook: 'https://yourapp.com/webhook',
  schedule: { frequency: 'daily' },
  tags: ['report'],
});
```