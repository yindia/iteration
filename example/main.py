from typing import TypedDict, List
from enum import Enum
from modules.task import task, TaskType, TaskSpec
from modules.workflow import Workflow, WorkflowSpec

@task(TaskSpec(
    name="hello_world",
    type=TaskType.PYTHON,
    description="This is a test task",
    dependencies="This is a test task",
    metadata={  
        "author": "Yuvraj Singh",
        "version": "1.0.0",
    },
    base_image="python:3.12",
    entrypoint="python",
    args=[],
    env={}
))
def hello_world(a : int, b : int) -> bool:
    print("Hello, World!")
    return True


@task(TaskSpec(
    name="hello_world",
    type=TaskType.PYTHON,
    description="This is a test task",
    dependencies="This is a test task",
    metadata={  
        "author": "Yuvraj Singh",
        "version": "1.0.0",
    },
    base_image="python:3.12",
    entrypoint="python",
    args=[],
    env={}
))
def hello_world_2(a : int, b : int) -> bool:
    print("Hello, World!")
    return True


@Workflow(WorkflowSpec(
    name="hello_world",
    description="This is a test workflow",
    metadata={
        "author": "Yuvraj Singh",
        "version": "1.0.0",
    },
))
def hello_world_workflow(a : int, b : int)-> bool:
    return hello_world(a=a, b=b) or hello_world(a=a, b=b)


if __name__ == "__main__":
    hello_world()
