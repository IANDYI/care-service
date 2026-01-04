# Care Service

Microservice for managing baby care records, measurements, and alerts. Handles baby registration, measurement logging (feeding, temperature, weight, diaper changes), and publishes alerts to RabbitMQ when critical measurements are detected.

## What It Does

- **Baby Management**: Create and retrieve baby records with parent ownership
- **Measurements**: Log feeding sessions, temperature readings, weight, and diaper changes
- **Safety Monitoring**: Automatically calculates safety status (green/yellow/red) for measurements
- **Alerts**: Publishes alerts to RabbitMQ when red status measurements are detected
- **Baby Creation Consumer**: Listens to RabbitMQ for baby creation requests from the identity service

## Architecture

The service follows a clean architecture pattern:
- **Domain**: Core business logic and entities (`internal/core/domain/`)
- **Services**: Business logic implementation (`internal/core/services/`)
- **Adapters**: HTTP handlers, repositories, middleware (`internal/adapters/`)
- **Ports**: Interfaces for dependency injection (`internal/core/ports/`)

## API Endpoints

All endpoints except health checks require JWT authentication via the `Authorization: Bearer <token>` header.

### Health & Metrics

- `GET /health` - General health check
- `GET /health/ready` - Readiness probe (checks database connectivity)
- `GET /health/live` - Liveness probe
- `GET /metrics` - Prometheus metrics

### Baby Management

- `POST /babies` - Create a baby (ADMIN only)
  ```json
  {
    "last_name": "Smith",
    "room_number": "101",
    "parent_user_id": "550e8400-e29b-41d4-a716-446655440000"
  }
  ```

- `GET /babies` - List babies (ADMIN: all, PARENT: owned only)
- `GET /babies/{baby_id}` - Get baby by ID (ADMIN: any, PARENT: owned only)

### Measurements

- `POST /babies/{baby_id}/measurements` - Create measurement (PARENT: owned only, ADMIN cannot create)
- `GET /babies/{baby_id}/measurements` - List measurements (supports `?type=` and `?limit=` query params)
- `GET /measurements/{measurement_id}` - Get measurement by ID
- `DELETE /measurements/{measurement_id}` - Delete measurement (PARENT: only own measurements)

### Measurement Types

**Feeding** (`type: "feeding"`):
- Bottle: `feeding_type: "bottle"`, `volume_ml: 120`
- Breast: `feeding_type: "breast"`, `side: "left"|"right"|"both"`, `position: "cross_cradle"|"cradle"|"football"|"side_lying"|"laid_back"`, `duration` or `left_duration`/`right_duration` in seconds

**Temperature** (`type: "temperature"`):
- `value_celsius: 37.2` or `value: 37.2`
- Safety status: Green (36.5-37.5°C), Yellow (36.0-36.5 or 37.5-38.0°C), Red (<36.0 or >38.0°C)

**Weight** (`type: "weight"`):
- `value: 3500` (in grams)

**Diaper** (`type: "diaper"`):
- `diaper_status: "dry"|"wet"|"dirty"|"both"`

## RabbitMQ Integration

### Baby Creation Consumer

The service includes a built-in consumer that listens to the `baby.creation.requests` queue. When the identity service publishes a baby creation request, the consumer automatically creates the baby record.

**Queue**: `baby.creation.requests`

**Message Format**:
```json
{
  "last_name": "Smith",
  "room_number": "101",
  "parent_user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

The consumer runs in the same process as the HTTP server and processes messages asynchronously.

### Alert Publisher

When a measurement with red safety status is created, the service publishes an alert to the `baby_alerts` queue:

```json
{
  "baby_id": "550e8400-e29b-41d4-a716-446655440000",
  "measurement": {
    "id": "...",
    "type": "temperature",
    "value": 38.5,
    "safety_status": "red"
  },
  "timestamp": "2024-01-15T10:30:00Z",
  "alert_type": "critical_measurement",
  "safety_status": "red",
  "severity": "high"
}
```

## Database Schema

The service auto-creates tables on startup (see `init.sql`). Main tables:

- `babies`: Baby records with parent ownership
- `measurements`: Measurement records with type-specific fields

## Monitoring

Prometheus metrics are exposed at `/metrics`. The service tracks:
- HTTP request duration and count
- Database operation metrics
- RabbitMQ publish/consume metrics

Health endpoints are compatible with OpenShift/Kubernetes probes:
- Liveness: `/health/live`
- Readiness: `/health/ready`

## Security

- All API endpoints (except health) require JWT authentication
- JWT tokens are validated using the public key from the identity service
- Role-based access control (RBAC):
  - **ADMIN**: Can create babies, view all babies and measurements
  - **PARENT**: Can only view/access their own babies, can create/delete measurements for their babies
- Parent ownership is enforced at the service layer
