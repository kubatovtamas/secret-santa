# Secret Santa Web Application Plan

## Project Goal:
Develop a "Secret Santa" web application for creating and managing gift exchange rooms without user accounts.

## Tech Stack:
- Backend: Go (Fiber framework)
- Frontend: HTML with HTMX
- Database: PostgreSQL
- CSS Framework: TBD (simple and low-overhead)

## Database Schema:
- PostgreSQL for room and participant management.
- Include timestamps, participant assignment, and dual room passwords.

## Functional Requirements:
- Password protection for rooms and participants.
- Admin password for room creators and join password for participants.
- Rate limiting on API endpoints.
- Automated participant matching and email notifications.
- Max room capacity set to 50.

## Main Algorithm:
- [Explainer](https://www.youtube.com/watch?v=GhnCj7Fvqt0)
- Create a list of structs with two int values: "You are X" and "You gift X" for each participant.
- Randomly shuffle the list.
- Split the structs into "You are X" and "You gift X".
- Shift the "You are X" part right by one position.
- Re-combine the structs. Each participant gets a struct, assigning them someone to gift.

## API Endpoints:
- POST `/room/create` - Create a room with a password.
- POST `/room/join` - Join a room with its password.
- GET `/room/list` - List all rooms.
- GET `/room/{id}/participants` - List participants (password required).
- DELETE `/room/{id}` - Delete a room (admin or password required).
- DELETE `/participant/{id}` - Delete a participant (admin or password required).

## HTML Pages:
- `index.html` - Homepage and room listing.
- `create-room.html` - Room creation form.
- `join-room.html` - Room joining form.
- `room-list.html` - List of rooms.
- `room-detail.html` - Room details and participant list.
- `admin.html` - Admin management interface.

## Security Measures:
- Hashed password storage and HTTPS.
- CSRF and rate limiting: Fiber
- Input validation for email formats.
- Email encryption in the database.
- SQLx parameterized queries for data sanitization.

## Deployment:
- Using Railway for hosting the Fiber app and the PostgreSQL instance.
