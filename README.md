# Task Service - Case Study

New Release [TBD]
- Versioning
- Divide control plane and data plane 
  - Replace Revierqueue with k8s controller
  - Introduce Agent 

<a href="https://vimeo.com/1014374705">
  <img src="https://markdown-videos-api.jorgenkh.no/url?url=https%3A%2F%2Fvimeo.com%2F1014374705" alt="Task Service " title="Task Service " width="100%" height="100%"/>
</a>

## Table of Contents
- [Quick Start](#quick-start)
- [Manual Setup](#manual-setup)
- [Project Structure](#project-structure)
- [Architecture](#architecture)
- [Database Schema](#database-schema)
- [API Documentation](#api-documentation)
- [CLI Usage](#cli-usage)
- [Additional Information](#additional-information)
- [Plugin Model](#plugin-model)
- [Testing in Kubernetes with Kind](#testing-in-kubernetes-with-kind)
- [Improvement](https://github.com/yindia/iteration/tree/postgres?tab=readme-ov-file#future-improvements)

## Quick Start
To quickly get started with the Task Service, use the following command:

```bash
make bootstrap

docker-compose -f docker-compose.demo.yaml up
```

This will start all the necessary services, including the server, database, and worker.

## Manual Setup

For a more detailed setup process, follow these steps:

### 1. Development Environment Setup
Install Pixi and activate the shell:
```bash
make bootstrap

# Run Database 
docker-compose up -d

# Install river
go install github.com/riverqueue/river/cmd/river@latest

# Run River migration  (It will create the river resource in the database)
river migrate-up --database-url "postgres://admin:admin@127.0.0.1:5432/tasks?sslmode=disable"
```


### 2. Server (Control Plane)
Start the server:
```bash
make run-server
```
Access at https://127.0.0.1:8080

### 3. CLI Tool
Build and test:
```bash
make build-cli
./bin/task-cli --help
```

### 4. Dashboard Setup
Install dependencies and build:
```bash
npm install
npm run dev
```
Access at https://127.0.0.1:3000

<img width="1548" alt="Screenshot 2024-09-29 at 4 22 37 PM" src="https://github.com/user-attachments/assets/7ec5f60a-e51d-44f7-bd55-9d9f761252a6">

### 5. Worker (Data Plane)
Start worker instances:
```bash
./bin/task-cli serve -n 10
```

## Project Structure
```
task/
├── cmd/
│   ├── cli/            # CLI for task management
│   └── server/         # Server entry point
├── pkg/
│   ├── config/         # Configuration management
│   ├── gen/            # GRPC generated code
│   ├── plugins/        # Plugin model
│   ├── worker/         # Worker code
│   └── x/              # Utility functions         
├── idl/
│   └── proto/          # Protocol buffer definitions
├── clients/
│   └── dashboard/      # NextJS Dashboard
├── charts/
│   └── task/         # Helm charts for deployment
├── server/
│   ├── repository/            # Database ORM
│   └── root/         # Server Root 
│   └── route/         # All Server Routes
└── docs/               # Documentation files
```

## Architecture

The Task Service follows a distributed architecture with separate components for the control plane and data plane. Here's a high-level overview of the system:

```mermaid
graph TD
    %% Clients
    A[Dashboard Client] -->|Sends Request| B(Server)
    C[CLI Client] -->|Sends Request| B(Server)

    %% Server and its connections
    B(Server) -->|Reads/Writes| D[(PostgreSQL Database)]
    B(Server) -->|Publishes Message| E(RiverQueue)

    %% RabbitMQ and Worker
    E(RiverQueue) -->|Sends Message| F(Worker)
    F(Worker) -->|Consumes Data| G[Executes Work]

    %% Optional back-and-forth communication if needed
    F(Worker) -->|Update Status| B[(Server)]
```

This architecture allows for:
- Separation of concerns between the control plane (server) and data plane (workers)
- Scalability of worker nodes to handle increased workloads
- Asynchronous task execution through message queuing
- Real-time status updates from workers to the server


### Database Operations

The server interacts with the database for persistent storage of tasks and their history. Here's a summary of the database operations:

1. **Read Operations**
   - Get Task by ID
     - Purpose: Retrieve details of a specific task
     - Frequency: On-demand, triggered by API requests
   - List All Tasks
     - Purpose: Retrieve a list of all tasks
     - Frequency: On-demand, typically for dashboard or reporting
   - List Task History
     - Purpose: Retrieve the status change history of a specific task
     - Frequency: On-demand, for detailed task analysis

2. **Write Operations**
   - Create New Task
     - Purpose: Store a newly created task
     - Frequency: Each time a new task is submitted
   - Update Task Status
     - Purpose: Modify the status of an existing task
     - Frequency: As task states change (e.g., from queued to running to completed)
   - Create Task History Entry
     - Purpose: Log task status changes and creation events
     - Frequency: On task creation and each status change


### Database Schema

The Task Service uses a PostgreSQL database to store task and task history information. Below is an Entity-Relationship Diagram (ERD) representing the database schema:

```mermaid
erDiagram
    %% Task Model
    TASK {
        int id PK
        string name
        int type
        int status
        jsonb payload
        int retries
        int priority
        timestamp created_at
    }
    
    %% TaskHistory Model
    TASK_HISTORY {
        int id PK
        int task_id FK
        int status
        string details
        timestamp created_at
    }

    %% Relationships
    TASK ||--o{ TASK_HISTORY : has

    %% Indexes (described as comments)
    %% Indexes for TASK
    %% - idx_type_status (type, status)
    %% - idx_created_at (created_at)
    %% - idx_status_created_at (status, created_at)

    %% Indexes for TASK_HISTORY
    %% - idx_task_id_created_at (task_id, created_at)
```

Note:  Ideally, we should create separate tables for tasks 📝 and task executions ⚙️. When a task is created, it should be added to the task table. Upon triggering an execution, a corresponding entry should be created in the execution table, and the execution data should be published to the PostgreSQL queue for processing 📬. This way, the task status remains unchanged, and only the execution status is updated in the execution table ✅.


### Table Descriptions

1. **TASK**
   - Stores information about individual tasks
   - `id`: Unique identifier for the task (Primary Key)
   - `name`: Name of the task
   - `type`: Type of the task (e.g., send_email, run_query)
   - `status`: Current status of the task (e.g., pending, running, completed)
   - `payload`: JSON object containing task-specific parameters
   - `retries`: Number of retry attempts for the task
   - `priority`: Priority level of the task
   - `created_at`: Timestamp of task creation

2. **TASK_HISTORY**
   - Tracks the history of status changes for tasks
   - `id`: Unique identifier for the history entry (Primary Key)
   - `task_id`: Foreign Key referencing the TASK table
   - `status`: Status of the task at the time of the history entry
   - `details`: Additional details about the status change
   - `created_at`: Timestamp of the history entry creation

### Relationships

- One TASK can have many TASK_HISTORY entries (one-to-many relationship)

### Indexes

To optimize query performance, the following indexes are implemented:

1. **TASK table**
   - `idx_type_status`: Composite index on `type` and `status` columns
   - `idx_created_at`: Index on `created_at` column
   - `idx_status_created_at`: Composite index on `status` and `created_at` columns

2. **TASK_HISTORY table**
   - `idx_task_id_created_at`: Composite index on `task_id` and `created_at` columns

These indexes improve the efficiency of common queries such as filtering tasks by type and status, sorting by creation time, and retrieving task history.


### Worker/Data Plane Process

The worker process follows a specific flow for task execution and error handling. Here's a detailed view of the worker's operation:

```mermaid
graph TD
    A[Receive Message] --> B{Update Status: RUNNING}
    B -->|Success| C[Run Task]
    B -->|Failure| D[Log Error]
    D --> K[Move to Next Message]

    C --> E{Task Execution}
    E -->|Success| F[Update Status: SUCCEEDED]
    E -->|Failure| G[Retry Logic]

    G --> H{Retry Attempt <= 3?}
    H -->|Yes| I[Backoff]
    I --> J[Update Status: RETRYING]
    J --> C
    H -->|No| K[Update Status: FAILED]

    F --> L[Move to Next Message]
    K --> L
```

Reconciliation Job (Run in every 10 minutes) as background job

```mermaid
graph TD
    %% Reconciliation Job Flow
    subgraph Reconciliation Job
        M[Start Reconciliation Job] --> N[Get List of Stuck Jobs]
        N --> O{Jobs Found?}
        O -->|Yes| P[Update Status: Queued]
        P --> Q[Enqueue Message to River Queue]
        O -->|No| R[End Reconciliation Job]
        Q --> R
    end
```

## API Documentation
- [Proto Docs](https://buf.build/evalsocket/cloud)
- [Studio](https://buf.build/studio/evalsocket/cloud/cloud.v1.TaskManagementService/CreateTask)

## CLI Usage

### Task Management

The Task Service CLI provides several commands to manage tasks. Here's a detailed overview of each command and its available flags:

#### Create a Task

Create a new task with the specified name, type, and parameters.

```bash
task-cli task create [task name] --type [task type] --parameter [key=value]
```

Flags:
- `--type`, `-t`: Type of the task (e.g., send_email, run_query)
- `--parameter`, `-p`: Additional parameters for the task as key=value pairs (can be used multiple times)

Example:
```bash
task-cli task create "Send Newsletter" --type send_email --parameter recipient=user@example.com --parameter subject="Weekly Update"
```

#### Get Task Details

Retrieve and display the details of a specific task by its ID.

```bash
task-cli  task get --id [task ID] [flags]
```

Flags:
- `--id`, `-i`: ID of the task (required)
- `--output`, `-o`: Output format (table, json, yaml) (default: "table")

Example:
```bash
task-cli  task get --id 123 --output json
```

#### Get Task History

Retrieve and display the history of a specific task by its ID.

```bash
task-cli history --id [task ID] [flags]
```

Flags:
- `--id`, `-i`: ID of the task (required)
- `--output`, `-o`: Output format (table, json, yaml) (default: "table")

Example:
```bash
task-cli  history --id 123 --output yaml
```

#### List All Tasks

Retrieve and display a list of all tasks.

```bash
task-cli task list [flags]
```

Flags:
- `--output`, `-o`: Output format (table, json, yaml) (default: "table")
- `--pageNumber`, `-n`: Page number for pagination (default: 1)
- `--pageCount`, `-c`: Number of items per page (default: 30)

Examples:
```bash
task-cli task list
task-cli task list --output json
task-cli task list --pageNumber 2 --pageCount 20
```

#### Task Status

Retrieve the status counts of all tasks in the system.

```bash
task-cli task status
```

Aliases: `s`, `stat`

Example:
```bash
task-cli task status
task-cli task s
```

This command will display the count of tasks for each status (e.g., PENDING, RUNNING, SUCCEEDED, FAILED).

#### End-to-End Testing

Run end-to-end tests against the system to verify its functionality.

```bash
task-cli end2end [flags]
```

Flags:
- `--num-tasks`, `-n`: Number of tasks to create for the test (default: 100, max: 100)

Example:
```bash
task-cli end2end
task-cli end2end -n 50
```

This command will:
1. Create the specified number of tasks (default 100)
2. Monitor the tasks' completion status for up to 3 minutes
3. Display progress every 5 seconds
4. Report the final result (success or partial completion)

The test creates a mix of "run_query" and "send_email" task types to simulate a realistic workload.


### Global Flags

The following flag is available for all task commands:

- `--log-level`: Set the logging level (default: "error")
- `--address`: Control Plane Address (default: "http://127.0.0.1:8080")

Example:
```bash
task-cli task list --log-level debug
```

### Output Formats

All commands that display task information support three output formats:

- `table`: Displays the information in a formatted table (default)
- `json`: Outputs the data in JSON format
- `yaml`: Outputs the data in YAML format

Use the `--output` or `-o` flag to specify the desired format.

## Additional Information
- Control plane (server) manages task creation, scheduling, and status updates
- Data plane (workers) executes tasks (Currently part of same binary)
- RiverQueue used for communication between control and data planes using postgres as queue backend
- Explore the UI or CLI to create and manage tasks

## Plugin Model

The Task Service uses a plugin-based architecture to allow for extensibility and customization of task execution. This model enables users to create their own task types and implement custom logic for task execution.

### How It Works

1. **Plugin Interface**: All plugins must implement the `Plugin` interface defined in `@pkg/plugins/plugins.go`. This interface requires a `Run` method:

   ```go
   type Plugin interface {
       Run(parameters map[string]string) error
   }
   ```

2. **Plugin Registration**: Plugins are registered in the `NewPlugin` function in `@pkg/plugins/plugins.go`. This function acts as a factory, creating the appropriate plugin based on the task type:

   ```go
   func NewPlugin(pluginType string) (Plugin, error) {
       switch pluginType {
       case email.PLUGIN_NAME:
           return &email.Email{}, nil
       case query.PLUGIN_NAME:
           return &query.Query{}, nil
       // Add more plugin types here
       default:
           return nil, fmt.Errorf("unknown plugin type: %s", pluginType)
       }
   }
   ```

3. **Custom Plugin Implementation**: Users can create their own plugins by implementing the `Plugin` interface. For example, the `Email` plugin in `@pkg/email/email.go`:

   ```go
   var PLUGIN_NAME = "send_email"
   type Email struct {}

   func (e *Email) Run(parameters map[string]string) error {
       // Implementation of email sending logic
       return nil
   }
   ```

4. **Task Execution**: When a task is executed, the system uses the `NewPlugin` function to create the appropriate plugin based on the task type. It then calls the `Run` method of the plugin, passing any necessary parameters.

### Creating a New Plugin

To create a new plugin:

1. Create a new package in the `@pkg/plugins` directory for your plugin.
2. Implement the `Plugin` interface in your new package.
3. Add your plugin to the `NewPlugin` function in `@pkg/plugins/plugins.go`.

This modular approach allows for easy extension of the Task Service with new task types and functionalities.


## Testing in Kubernetes with Kind

This section guides you through setting up and testing the Task Service in a local Kubernetes cluster using Kind (Kubernetes in Docker) and Helm charts.

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Helm](https://helm.sh/docs/intro/install/)

### Setup

1. Create a Kind cluster:

```bash
kind create cluster --name task-service
```

2. Set kubectl context to the new cluster:

```bash
kubectl cluster-info --context kind-task-service
```

3. Add the necessary Helm repositories:

```bash
make helm
```

### Deploy Task Service

Install the Task Service Helm chart:

```bash
helm install task-service ./charts/task  -n task
```

### Verify Deployment

Check that all pods are running:

```bash
kubectl get pods
```

## Port Forward
```
kubectl port-forward service/task 80 -n task
```

### Access the Service

1. Port-forward the Task Service:

```bash
kubectl port-forward -n task svc/task 8080:80
```

2. Access the service at `http://127.0.0.1:8080`

3. Use CLI to verify the connection:
```bash
./bin/task-cli task l --address http://127.0.0.1:8080
```

### Clean Up

To delete the Kind cluster and all resources:

```bash
kind delete cluster --name task-service
```

This setup allows you to test the entire Task Service stack, including the server, workers, and dependencies, in a local Kubernetes environment. It's an excellent way to validate the Helm charts and ensure everything works together as expected in a Kubernetes setting.


## Future Improvements

As we continue to evolve the Task Service, we are exploring several enhancements to improve its scalability, reliability, and management. 

### Kubernetes-Native Task Execution

We are considering leveraging Kubernetes Custom Resource Definitions (CRDs) and custom controllers to manage task execution. This approach would enable us to fully utilize Kubernetes' scheduling and scaling capabilities.

#### High-Level Architecture

```mermaid
graph TD
    %% Clients
    A[Dashboard Client] -->|Sends Request| B(Server)
    C[CLI Client] -->|Sends Request| B(Server)

    %% Control Plane
    subgraph Control Plane
        B(Server) -->|Reads/Writes| D[(PostgreSQL Database)]
    end

    %% Data Plane
    subgraph Data Plane
        E[Agent] -->|Initiates Connection| B[Server]
        E -->|Creates CRD| H[CRD]
        F[Controller] -->|Watches CRD| H
        F -->|Creates Pod for Task| I[Pod]
        I -->|Executes Task| J[Task Execution]
        F -->|Sends Status Update| B
    end
```

In this architecture:

1. Our agent initiates a streaming connection with the control plane and listens for events.
2. When a new task is created, the control plane generates an event for the agent.
3. Upon receiving the event, the agent creates a Custom Resource Definition (CRD) for the task in Kubernetes.
4. A custom Worker Controller watches for these CRDs and creates pods to execute the tasks.
5. Each task runs in its own pod, allowing for improved isolation and resource management.
6. The Worker Controller monitors task execution and sends status updates back to the server.


#### Design Advantages

- **Separation of Concerns**: The customer does not need to open a port; our agent initiates the connection, and only the agent has permission to create resources inside the Data Plane.
- **Single Point of Setup**: Only the agent is required to set up the Data Plane, creating the necessary resources such as the controller, CRD, and other components.
- **Multiple Data Planes**: Customers can run multiple Data Planes with one Control Plane based on their requirements (from bare metal to any cloud). In the future, we can add functionality to route tasks to specific Data Planes as needed.
- **Security**: No sensitive information is stored in the Control Plane; we only retain metadata, ensuring enhanced security.
- **Infinite Scalability**: The architecture supports scaling as needed to accommodate varying workloads.
- **Co-location Flexibility**: Customers can run both the Data Plane and Control Plane together inside their VPC for easier management.
- **Secure Storage**: All input parameters are stored as S3 objects, with only references to these objects kept in the metadata, optimizing storage usage.
