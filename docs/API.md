# API Reference

This document describes the RESTful API endpoints for the TikTok-like application.

## Base URL

```
Production: https://api.example.com
Development: http://localhost:8080
```

## API Versioning

The API uses URL-based versioning:
- **v1**: Legacy API (maintained for backward compatibility)
- **v2**: Recommended for new integrations

---

## Authentication

Most endpoints require JWT authentication. Include the token in the `Authorization` header:

```
Authorization: Bearer <your_jwt_token>
```

---

## User Service (`/api/v1/users`)

### Authentication

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/v1/users/register` | Register a new user | No |
| POST | `/api/v1/users/login` | User login | No |

### User Management

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | `/api/v1/users/:id` | Get user profile | Yes |
| PUT | `/api/v1/users/:id` | Update user profile | Yes |
| DELETE | `/api/v1/users/:id` | Delete user account | Yes |
| GET | `/api/v1/users/` | Query users | Yes |
| POST | `/api/v1/users/check` | Check if user exists | Yes |

### Email Verification

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/v1/users/verification/send` | Send verification code | No |
| POST | `/api/v1/users/verification/verify` | Verify email code | No |

### Avatar Management

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/v1/users/:id/avatar/upload-url` | Get avatar upload URL | Yes |
| PUT | `/api/v1/users/:id/avatar` | Update user avatar | Yes |

---

## Video Service

### V2 API (Recommended) - `/api/v2`

#### Videos

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | `/api/v2/videos/` | List videos | Yes |
| GET | `/api/v2/videos/feed` | Get video feed | Yes |
| GET | `/api/v2/videos/popular` | Get popular videos | Yes |
| GET | `/api/v2/videos/recommend` | Get recommended videos | Yes |
| GET | `/api/v2/videos/search` | Search videos | Yes |
| DELETE | `/api/v2/videos/:video_id` | Delete a video | Yes |
| GET | `/api/v2/videos/:video_id/analytics` | Get video analytics | Yes |
| POST | `/api/v2/videos/:video_id/visit` | Record video visit | No |
| GET | `/api/v2/videos/:video_id/visit-count` | Get visit count | No |
| GET | `/api/v2/videos/hot/ranking` | Get hot video ranking | No |
| POST | `/api/v2/videos/batch` | Batch video operations | Yes |

#### Video Publishing

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/v2/publish/start` | Start video upload | Yes |
| POST | `/api/v2/publish/uploading` | Upload video chunk | Yes |
| POST | `/api/v2/publish/complete` | Complete video upload | Yes |
| POST | `/api/v2/publish/cancel` | Cancel video upload | Yes |
| GET | `/api/v2/publish/progress` | Get upload progress | Yes |
| POST | `/api/v2/publish/resume` | Resume upload | Yes |

#### Favorites

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | `/api/v2/favorites/` | List favorites | Yes |
| POST | `/api/v2/favorites/` | Create favorite folder | Yes |
| DELETE | `/api/v2/favorites/:favorite_id` | Delete favorite folder | Yes |
| GET | `/api/v2/favorites/:favorite_id/videos` | List videos in favorite | Yes |
| POST | `/api/v2/favorites/:favorite_id/videos` | Add video to favorite | Yes |
| DELETE | `/api/v2/favorites/:favorite_id/videos/:video_id` | Remove video from favorite | Yes |

#### Watch History

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | `/api/v2/history/` | Get watch history | Yes |
| POST | `/api/v2/history/` | Add to history | Yes |
| DELETE | `/api/v2/history/` | Clear all history | Yes |
| DELETE | `/api/v2/history/:history_id` | Delete history item | Yes |

#### Notifications

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | `/api/v2/notifications/` | List notifications | Yes |
| GET | `/api/v2/notifications/unread-count` | Get unread count | Yes |
| POST | `/api/v2/notifications/:notification_id/read` | Mark as read | Yes |

---

## Interaction Service (`/api/v1`)

### Likes

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/v1/videos/:video_id/likes` | Like a video | Yes |
| DELETE | `/api/v1/videos/:video_id/likes` | Unlike a video | Yes |
| GET | `/api/v1/likes/` | Get user's liked videos | Yes |
| POST | `/api/v1/likes/batch-status` | Get batch like status | Yes |

### Comments

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | `/api/v1/videos/:video_id/comments` | List comments | No |
| POST | `/api/v1/videos/:video_id/comments` | Create comment | Yes |
| DELETE | `/api/v1/videos/:video_id/comments/:comment_id` | Delete comment | Yes |

---

## Relation Service (`/api/v1`)

### User Relations

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/v1/users/:user_id/follow` | Follow a user | Yes |
| DELETE | `/api/v1/users/:user_id/follow` | Unfollow a user | Yes |
| GET | `/api/v1/users/:user_id/followers` | Get user's followers | Yes |
| GET | `/api/v1/users/:user_id/following` | Get user's following | Yes |
| GET | `/api/v1/users/:user_id/friends` | Get user's friends | Yes |

### Current User Shortcuts

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/v1/relations/follow` | Follow action | Yes |
| GET | `/api/v1/relations/followers` | My followers | Yes |
| GET | `/api/v1/relations/following` | My following | Yes |
| GET | `/api/v1/relations/friends` | My friends | Yes |

---

## Response Format

All responses follow this standard format:

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

### Error Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 10001 | Invalid parameter |
| 10002 | Unauthorized |
| 10003 | Forbidden |
| 10004 | Not found |
| 10005 | Internal error |

---

## Pagination

For list endpoints, use these query parameters:

| Parameter | Type | Description |
|-----------|------|-------------|
| page_num | int | Page number (default: 1) |
| page_size | int | Items per page (default: 10, max: 100) |

Response includes pagination metadata:

```json
{
  "code": 0,
  "message": "success",
  "data": [...],
  "total": 100,
  "page": 1,
  "size": 10
}
```
