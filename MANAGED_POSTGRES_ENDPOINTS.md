# Managed Postgres API Endpoints

All endpoints are prefixed with the base URL and require `Authorization: Bearer <token>` header.

---

## Shared Types

```
Organization {
  id:   string
  name: string
  slug: string
}

ManagedCluster {
  id:             string
  name:           string
  region:         string
  status:         string
  plan:           string
  disk:           int
  replicas:       int
  organization:   Organization
  ip_assignments: { direct: string }
  attached_apps:  [{ name: string, id: int }]
}

ManagedClusterCredentials {
  status:        string
  user:          string
  password:      string
  dbname:        string
  pgbouncer_uri: string
}

Backup {
  id:     string
  status: string
  type:   string
  start:  string
  stop:   string
}

User {
  name: string
  role: string
}

Database {
  name: string
}
```

---

## Clusters

### List Clusters

```
GET /api/v1/organizations/{org_slug}/postgres
```

Response:
```
{ data: []ManagedCluster }
```

### List Deleted Clusters

```
GET /api/v1/organizations/{org_slug}/postgres/deleted
```

Response:
```
{ data: []ManagedCluster }
```

### Get Cluster (by org)

```
GET /api/v1/organizations/{org_slug}/postgres/{id}
```

Response:
```
{
  data:        ManagedCluster
  credentials: ManagedClusterCredentials
}
```

### Get Cluster (by id)

```
GET /api/v1/postgres/{id}
```

Response:
```
{
  data:        ManagedCluster
  credentials: ManagedClusterCredentials
}
```

### Create Cluster

```
POST /api/v1/organizations/{org_slug}/postgres
```

Input:
```
{
  name:             string
  region:           string
  plan:             string
  org_slug:         string
  disk:             int
  postgis_enabled:  bool
  pg_major_version: string
}
```

Response:
```
{
  ok:     bool
  errors: { detail: string }
  data: {
    id:              string
    name:            string
    status:          string?
    plan:            string
    environment:     string?
    region:          string
    organization:    Organization
    replicas:        int
    disk:            int
    ip_assignments:  { direct: string }
    postgis_enabled: bool
  }
}
```

### Destroy Cluster

```
DELETE /api/v1/organizations/{org_slug}/postgres/{id}
```

Response: _none (204)_

---

## Regions

### List MPG Regions

```
GET /api/v1/organizations/{org_slug}/postgres/regions
```

Response:
```
{
  data: [{
    code:      string
    available: bool
  }]
}
```

---

## Users

### List Users

```
GET /api/v1/postgres/{id}/users
```

Response:
```
{ data: []User }
```

### Create User

```
POST /api/v1/postgres/{id}/users
```

Input:
```
{
  db_name:   string
  user_name: string
}
```

Response:
```
{
  connection_uri: string
  ok:             bool
  errors:         { detail: string }
}
```

### Create User (with role)

```
POST /api/v1/postgres/{id}/users
```

Input:
```
{
  user_name: string
  role:      "schema_admin" | "writer" | "reader"
}
```

Response:
```
{ data: User }
```

### Update User Role

```
PATCH /api/v1/postgres/{id}/users/{username}
```

Input:
```
{
  role: "schema_admin" | "writer" | "reader"
}
```

Response:
```
{ data: User }
```

### Delete User

```
DELETE /api/v1/postgres/{id}/users/{username}
```

Response: _none (204)_

### Get User Credentials

```
GET /api/v1/postgres/{id}/users/{username}/credentials
```

Response:
```
{
  data: {
    user:     string
    password: string
  }
}
```

---

## Databases

### List Databases

```
GET /api/v1/postgres/{id}/databases
```

Response:
```
{ data: []Database }
```

### Create Database

```
POST /api/v1/postgres/{id}/databases
```

Input:
```
{
  name: string
}
```

Response:
```
{ data: Database }
```

---

## Backups

### List Backups

```
GET /api/v1/postgres/{id}/backups
```

Response:
```
{ data: []Backup }
```

### Create Backup

```
POST /api/v1/postgres/{id}/backups
```

Input:
```
{
  type: string
}
```

Response:
```
{ data: Backup }
```

### Restore from Backup

```
POST /api/v1/postgres/{id}/restore
```

Input:
```
{
  backup_id: string
}
```

Response:
```
{ data: ManagedCluster }
```

---

## Attachments

### Create Attachment

```
POST /api/v1/postgres/{id}/attachments
```

Input:
```
{
  app_name: string
}
```

Response:
```
{
  data: {
    id:                 int
    app_id:             int
    managed_service_id: int
    attached_at:        string
  }
}
```

### Delete Attachment

```
DELETE /api/v1/postgres/{id}/attachments/{app_name}
```

Response:
```
{
  data: {
    message: string
  }
}
```
