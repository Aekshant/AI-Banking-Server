from fastapi import FastAPI
from pydantic import BaseModel

from app.llm import ask_llm
from app.tool_executor import execute_tool


app = FastAPI(title="Bank Agent Engine")


class ChatRequest(BaseModel):
    message: str


tools = [
    {
        "type": "function",
        "function": {
            "name": "check_account",
            "description": (
                "Get a customer's account balance, "
                "name and credit score."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "customer_id": {
                        "type": "string",
                        "description": "Customer ID"
                    }
                },
                "required": ["customer_id"],
            },
        },
    }
]


@app.post("/chat")
async def chat_endpoint(request: ChatRequest):

    messages = [
        {
            "role": "user",
            "content": request.message,
        }
    ]

    # STEP 1: Ask LLM what to do
    response = await ask_llm(
        messages=messages,
        tools=tools,
    )
    print("1response", response)
    assistant_message = response["message"]

    # STEP 2: Did the model request a tool?
    if assistant_message.get("tool_calls"):

        tool_call = assistant_message["tool_calls"][0]

        tool_name = tool_call["function"]["name"]
        arguments = tool_call["function"]["arguments"]

        # STEP 3: Execute the requested tool
        tool_result = await execute_tool(
            tool_name,
            arguments,
        )
        print("tool_result", tool_result)
        # STEP 4: Give the tool result back to the LLM
        messages.append(assistant_message)

        messages.append(
            {
                "role": "tool",
                "content": str(tool_result),
            }
        )

        # STEP 5: Ask LLM for final answer
        final_response = await ask_llm(
            messages=messages,
        )
        print("final_response", final_response)
        return {
            "response": final_response["message"]["content"]
        }

    # No tool required
    return {
        "response": assistant_message["content"]
    }