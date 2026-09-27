from ollama import chat

MODEL_NAME = "qwen3:4b"


async def ask_llm(messages: list, tools: list | None = None):

    response = chat(
        model=MODEL_NAME,
        messages=messages,
        tools=tools or [],
    )

    return response