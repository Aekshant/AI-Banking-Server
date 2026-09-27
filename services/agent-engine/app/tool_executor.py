from app.tools import check_account


async def execute_tool(tool_name: str, arguments: dict):

    if tool_name == "check_account":
        return await check_account(
            customer_id=arguments["customer_id"]
        )

    raise ValueError(f"Unknown tool: {tool_name}")