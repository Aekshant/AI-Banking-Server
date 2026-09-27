from app.banking_client import get_account_profile


async def check_account(customer_id: str) -> dict:
    return await get_account_profile(customer_id)