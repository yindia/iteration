package route

import (
	"context"
	"fmt"
	"log"
	"os"
	v1 "task/pkg/gen/cloud/v1"
	"task/pkg/gen/cloud/v1/cloudv1connect"
	"task/pkg/x"
	interfaces "task/server/repository/interface"
	"task/server/repository/model/task"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"sync"

	connect "connectrpc.com/connect"
	protovalidate "github.com/bufbuild/protovalidate-go"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	logPrefix           = "TaskServer: "
	defaultTaskPriority = 0
	defaultTaskRetries  = 0
)

// TaskServer represents the server handling task-related requests.
// It implements the cloudv1connect.TaskManagementServiceHandler interface.
type TaskServer struct {
	taskRepo         interfaces.TaskRepo
	historyRepo      interfaces.TaskHistoryRepo
	logger           *log.Logger
	validator        *protovalidate.Validator
	metrics          *taskMetrics
	workflowRepo     interfaces.WorkflowRepo
	executionRepo    interfaces.ExecutionRepo
	channel          chan task.Task
	maxWorkers       int
	clientHeartbeats sync.Map
	heartbeatTimeout time.Duration
}

type taskMetrics struct {
	createTaskCounter       prometheus.Counter
	getTaskCounter          prometheus.Counter
	getTaskHistoryCounter   prometheus.Counter
	updateTaskStatusCounter prometheus.Counter
	listTasksCounter        prometheus.Counter
	errorCounter            *prometheus.CounterVec
	taskDuration            *prometheus.HistogramVec
}

func newTaskMetrics() *taskMetrics {
	return &taskMetrics{
		createTaskCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "task_create_total",
			Help: "The total number of create task requests",
		}),
		getTaskCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "task_get_total",
			Help: "The total number of get task requests",
		}),
		getTaskHistoryCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "task_get_history_total",
			Help: "The total number of get task history requests",
		}),
		updateTaskStatusCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "task_update_status_total",
			Help: "The total number of update task status requests",
		}),
		listTasksCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "task_list_total",
			Help: "The total number of list tasks requests",
		}),
		errorCounter: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "task_errors_total",
			Help: "The total number of errors across all task operations",
		}, []string{"operation"}),
		taskDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "task_duration_seconds",
			Help:    "The duration of task operations in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
	}
}

// NewTaskServer creates and returns a new instance of TaskServer.
// It initializes the validator, sets up the logger, and configures metrics.
// The maxWorkers parameter can be configured to control the number of concurrent workers.
func NewTaskServer(repo interfaces.TaskManagmentInterface) cloudv1connect.TaskManagementServiceHandler {
	// Initialize the validator for request validation
	validator, err := protovalidate.New()
	if err != nil {
		log.Fatalf("Failed to initialize validator: %v", err)
	}

	maxWorkers := 500 // Configurable maximum number of concurrent workers
	server := &TaskServer{
		taskRepo:         repo.TaskRepo(),
		historyRepo:      repo.TaskHistoryRepo(),
		workflowRepo:     repo.WorkflowRepo(),
		executionRepo:    repo.ExecutionRepo(),
		logger:           log.New(os.Stdout, logPrefix, log.LstdFlags|log.Lshortfile),
		validator:        validator,
		metrics:          newTaskMetrics(),
		maxWorkers:       maxWorkers,
		heartbeatTimeout: 30 * time.Second, // Configurable timeout for heartbeats
	}

	server.logger.Println("TaskServer initialized successfully")
	return server
}

// CreateTask creates a new task, logs the operation, and returns the created task's ID.
// It also attempts to log the task creation in the history.
func (s *TaskServer) CreateTask(ctx context.Context, req *connect.Request[v1.CreateTaskRequest]) (*connect.Response[v1.CreateTaskResponse], error) {
	timer := prometheus.NewTimer(s.metrics.taskDuration.WithLabelValues("create_task"))
	defer timer.ObserveDuration()

	s.metrics.createTaskCounter.Inc()
	s.logger.Printf("Creating task: name=%s, type=%s", req.Msg.Name, req.Msg.GetType())

	// Validate the incoming request
	if err := s.validateRequest(req.Msg); err != nil {
		s.logger.Printf("CreateTask validation failed: %v", err)
		return nil, err
	}

	newTask := s.prepareNewTask(req.Msg)

	// Attempt to create the task in the repository
	createdTask, err := s.taskRepo.CreateTask(ctx, newTask)
	if err != nil {
		s.metrics.errorCounter.WithLabelValues("create_task").Inc()
		return nil, s.logError(err, "Failed to create task in repository")
	}

	s.logger.Printf("Task created successfully: id=%d", createdTask.ID)
	return connect.NewResponse(&v1.CreateTaskResponse{Id: int32(createdTask.ID)}), nil
}

// GetTask retrieves the status of a task by its ID.
func (s *TaskServer) GetTask(ctx context.Context, req *connect.Request[v1.GetTaskRequest]) (*connect.Response[v1.Task], error) {
	timer := prometheus.NewTimer(s.metrics.taskDuration.WithLabelValues("get_task"))
	defer timer.ObserveDuration()

	s.metrics.getTaskCounter.Inc()
	s.logger.Printf("Retrieving task: id=%d", req.Msg.Id)

	// Validate the incoming request
	if err := s.validateRequest(req.Msg); err != nil {
		return nil, err
	}

	// Fetch the task from the repository
	taskResponse, err := s.taskRepo.GetTaskByID(ctx, uint(req.Msg.Id))
	if err != nil {
		s.metrics.errorCounter.WithLabelValues("get_task").Inc()
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("task not found: %w", err))
	}

	s.logger.Printf("Task retrieved successfully: id=%d", req.Msg.Id)
	return connect.NewResponse(s.convertTaskToProto(taskResponse)), nil
}

// GetTaskHistory retrieves the history of a task by its ID.
func (s *TaskServer) GetTaskHistory(ctx context.Context, req *connect.Request[v1.GetTaskHistoryRequest]) (*connect.Response[v1.GetTaskHistoryResponse], error) {
	timer := prometheus.NewTimer(s.metrics.taskDuration.WithLabelValues("get_task_history"))
	defer timer.ObserveDuration()

	s.metrics.getTaskHistoryCounter.Inc()
	s.logger.Printf("Retrieving task history: id=%d", req.Msg.Id)

	// Validate the incoming request
	if err := s.validateRequest(req.Msg); err != nil {
		return nil, err
	}

	// Fetch the task history from the repository
	history, err := s.historyRepo.ListTaskHistories(ctx, uint(req.Msg.Id))
	if err != nil {
		s.metrics.errorCounter.WithLabelValues("get_task_history").Inc()
		return nil, s.logError(err, "Failed to retrieve task history: id=%d", req.Msg.Id)
	}

	protoHistory := s.convertTaskHistoryToProto(history)

	s.logger.Printf("Task history retrieved: id=%d, records=%d", req.Msg.Id, len(protoHistory))
	return connect.NewResponse(&v1.GetTaskHistoryResponse{History: protoHistory}), nil
}

// UpdateTaskStatus updates the status of a task and logs the operation.
func (s *TaskServer) UpdateTaskStatus(ctx context.Context, req *connect.Request[v1.UpdateTaskStatusRequest]) (*connect.Response[emptypb.Empty], error) {
	timer := prometheus.NewTimer(s.metrics.taskDuration.WithLabelValues("update_task_status"))
	defer timer.ObserveDuration()

	s.metrics.updateTaskStatusCounter.Inc()
	s.logger.Printf("Updating task status: id=%d, status=%s", req.Msg.Id, req.Msg.Status)

	// Validate the incoming request
	if err := s.validateRequest(req.Msg); err != nil {
		return nil, err
	}

	// Update the task status in the repository
	if err := s.taskRepo.UpdateTaskStatus(ctx, uint(req.Msg.Id), int(req.Msg.Status)); err != nil {
		s.metrics.errorCounter.WithLabelValues("update_task_status").Inc()
		return nil, s.logError(err, "Failed to update task status: id=%d", req.Msg.Id)
	}

	// Log the status update in the task history
	if err := s.createTaskStatusHistory(ctx, uint(req.Msg.Id), int(req.Msg.Status), req.Msg.Message); err != nil {
		s.logger.Printf("WARNING: Failed to create task status history: %v", err)
	}

	s.logger.Printf("Task status updated: id=%d", req.Msg.Id)
	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListTasks retrieves a list of tasks with pagination support.
func (s *TaskServer) ListTasks(ctx context.Context, req *connect.Request[v1.TaskListRequest]) (*connect.Response[v1.TaskList], error) {
	timer := prometheus.NewTimer(s.metrics.taskDuration.WithLabelValues("list_tasks"))
	defer timer.ObserveDuration()

	s.metrics.listTasksCounter.Inc()
	s.logger.Print("Retrieving list of tasks")

	// Validate the incoming request
	if err := s.validateRequest(req.Msg); err != nil {
		return nil, err
	}

	// Set default limit to 100 if not specified or invalid
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 100 // Default limit
	}

	// Set default offset to 0 if not specified or invalid
	offset := int(req.Msg.Offset)
	if offset < 0 {
		offset = 0 // Default offset
	}

	// Fetch the list of tasks from the repository
	tasks, err := s.taskRepo.ListTasks(ctx, limit, offset, int(req.Msg.GetStatus()), req.Msg.GetType())
	if err != nil {
		s.metrics.errorCounter.WithLabelValues("list_tasks").Inc()
		return nil, s.logError(err, "Failed to retrieve task list")
	}

	protoTasks := make([]*v1.Task, len(tasks))
	for i, task := range tasks {
		protoTasks[i] = s.convertTaskToProto(&task)
	}

	s.logger.Printf("Task list retrieved: count=%d", len(protoTasks))
	return connect.NewResponse(&v1.TaskList{Tasks: protoTasks}), nil
}

// GetStatus retrieves the count of tasks for each status.
func (s *TaskServer) GetStatus(ctx context.Context, req *connect.Request[v1.GetStatusRequest]) (*connect.Response[v1.GetStatusResponse], error) {
	timer := prometheus.NewTimer(s.metrics.taskDuration.WithLabelValues("get_status"))
	defer timer.ObserveDuration()

	s.metrics.getTaskCounter.Inc()
	s.logger.Print("Retrieving task status counts")

	// Validate the incoming request
	if err := s.validateRequest(req.Msg); err != nil {
		return nil, err
	}

	// Fetch the task status counts from the repository
	statusCounts, err := s.taskRepo.GetTaskStatusCounts(ctx)
	if err != nil {
		s.metrics.errorCounter.WithLabelValues("get_status").Inc()
		return nil, s.logError(err, "Failed to retrieve task status counts")
	}

	response := &v1.GetStatusResponse{
		StatusCounts: make(map[int32]int64),
	}

	for status, count := range statusCounts {
		response.StatusCounts[int32(status)] = count
	}

	s.logger.Printf("Task status counts retrieved successfully")
	return connect.NewResponse(response), nil
}

// Heartbeat handles client heartbeats to maintain connection status.
func (s *TaskServer) Heartbeat(ctx context.Context, req *connect.Request[v1.HeartbeatRequest]) (*connect.Response[v1.HeartbeatResponse], error) {
	s.clientHeartbeats.Store("clientID", req.Msg.Timestamp)
	return connect.NewResponse(&v1.HeartbeatResponse{}), nil
}

// PullEvents handles bidirectional streaming for task updates and assignments.
func (s *TaskServer) PullEvents(ctx context.Context, req *connect.Request[v1.PullEventsRequest], stream *connect.ServerStream[v1.PullEventsResponse]) error {
	ticker := time.NewTicker(10 * time.Second) // Trigger every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tasks, err := s.taskRepo.GetStalledTasks(ctx)
			if err != nil {
				s.logger.Printf("Error checking stalled tasks: %v", err)
				continue // Skip to the next tick on error
			}

			for _, t := range tasks {
				if err := stream.Send(&v1.PullEventsResponse{
					Work: &v1.WorkAssignment{
						AssignmentId: int64(t.ID),
						Task:         s.convertTaskToProto(&t),
					},
				}); err != nil {
					s.logger.Printf("Error sending task to client: %v", err)
					return err
				}
				if err := s.updateTaskStatus(ctx, uint(t.ID), v1.TaskStatusEnum_QUEUED, "Task is Queued"); err != nil {
					s.logger.Printf("Error updating task status: %v", err)
				}
			}
		case <-ctx.Done():
			return ctx.Err() // Exit if the context is done
		}
	}
}

// updateTaskStatus updates the task status and creates a history entry.
func (s *TaskServer) updateTaskStatus(ctx context.Context, taskID uint, status v1.TaskStatusEnum, message string) error {
	if err := s.taskRepo.UpdateTaskStatus(ctx, taskID, int(status)); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	if err := s.createTaskStatusHistory(ctx, taskID, int(status), message); err != nil {
		s.logger.Printf("WARNING: Failed to create task status history: %v", err)
	}

	return nil
}

// handleUpdateTaskStatus processes task status update requests.
func (s *TaskServer) handleUpdateTaskStatus(ctx context.Context, update *v1.UpdateTaskStatusRequest) error {
	if err := s.taskRepo.UpdateTaskStatus(ctx, uint(update.Id), int(update.Status)); err != nil {
		s.metrics.errorCounter.WithLabelValues("update_task_status").Inc()
		return fmt.Errorf("failed to update task status: id=%d, error: %w", update.Id, err)
	}

	if err := s.createTaskStatusHistory(ctx, uint(update.Id), int(update.Status), update.Message); err != nil {
		s.logger.Printf("WARNING: Failed to create task status history: %v", err)
	}

	s.logger.Printf("Task status updated: id=%d, new status=%d", update.Id, update.Status)
	return nil
}

// createTaskStatusHistory creates a new task history entry for the status update.
func (s *TaskServer) createTaskStatusHistory(ctx context.Context, taskID uint, status int, message string) error {
	_, err := s.historyRepo.CreateTaskHistory(ctx, task.TaskHistory{
		TaskID:  taskID,
		Status:  status,
		Details: message,
	})
	if err != nil {
		return fmt.Errorf("failed to create task history: %w", err)
	}
	return nil
}

// validateRequest validates the request using protovalidate.
// It returns an error if the message is not a valid protobuf message or fails validation.
func (s *TaskServer) validateRequest(msg interface{}) error {
	protoMsg, ok := msg.(protoreflect.ProtoMessage)
	if !ok {
		return s.logError(fmt.Errorf("msg is not a protoreflect.ProtoMessage"), "Invalid message type")
	}
	if err := s.validator.Validate(protoMsg); err != nil {
		s.logger.Printf("Request validation failed: %v", err)
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("validation failed: %w", err))
	}
	return nil
}

// prepareNewTask creates a new task.Task from the CreateTaskRequest.
// It handles the conversion of the payload to JSON and sets default values.
func (s *TaskServer) prepareNewTask(req *v1.CreateTaskRequest) task.Task {
	payloadJSON, err := x.ConvertMapToJson(req.Payload.Parameters)
	if err != nil {
		s.logger.Printf("WARNING: Failed to convert payload to JSON: %v", err)
	}

	newTask := task.Task{
		Name:        req.Name,
		Status:      int(v1.TaskStatusEnum_UNKNOWN),
		Description: req.Description,
		Type:        req.Type,
		Payload:     payloadJSON,
		Retries:     defaultTaskRetries,
		Priority:    defaultTaskPriority,
	}

	s.logger.Printf("Prepared new task: name=%s, type=%s", newTask.Name, newTask.Type)
	return newTask
}

// logTaskCreationHistory logs the task creation in the history.
// It creates a new TaskHistory entry with the initial QUEUED status.
func (s *TaskServer) logTaskCreationHistory(ctx context.Context, taskID uint) error {
	_, err := s.historyRepo.CreateTaskHistory(ctx, task.TaskHistory{
		TaskID:  taskID,
		Status:  int(v1.TaskStatusEnum_QUEUED),
		Details: "Task is scheduled",
	})
	if err != nil {
		return fmt.Errorf("failed to create task history for task ID %d: %v", taskID, err)
	}
	s.logger.Printf("Task creation history logged for task ID: %d", taskID)
	return nil
}

// convertTaskToProto converts a task model to a protobuf Task message.
func (s *TaskServer) convertTaskToProto(taskModel *task.Task) *v1.Task {
	jsonMap, err := x.ConvertJsonToMap(taskModel.Payload)
	if err != nil {
		s.logger.Printf("WARNING: Failed to convert task payload to map: %v", err)
	}

	return &v1.Task{
		Id:          int32(taskModel.ID),
		Name:        taskModel.Name,
		Description: taskModel.Description,
		Status:      v1.TaskStatusEnum(taskModel.Status),
		Priority:    int32(taskModel.Priority),
		Retries:     int32(taskModel.Retries),
		Payload:     &v1.Payload{Parameters: jsonMap},
		Type:        taskModel.Type,
	}
}

// logError logs the error message and returns a connect.Error.
// It ensures consistent error logging and error response creation.
func (s *TaskServer) logError(err error, message string, args ...interface{}) error {
	s.metrics.errorCounter.WithLabelValues("unknown").Inc()
	fullMessage := fmt.Sprintf(message, args...)
	s.logger.Printf("ERROR: %s: %v", fullMessage, err)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", fullMessage, err))
}

// convertTaskHistoryToProto converts task history models to protobuf TaskHistory messages.
func (s *TaskServer) convertTaskHistoryToProto(history []task.TaskHistory) []*v1.TaskHistory {
	protoHistory := make([]*v1.TaskHistory, len(history))
	for i, h := range history {
		protoHistory[i] = &v1.TaskHistory{
			Id:        int32(h.ID),
			Status:    v1.TaskStatusEnum(h.Status),
			CreatedAt: h.CreatedAt.Format(time.RFC3339),
			Details:   h.Details,
		}
	}
	return protoHistory
}
