import httpx


BANKING_SERVICE_URL = "http://127.0.0.1:8002"


async def get_account_profile(customer_id: str) -> dict:
    url = f"{BANKING_SERVICE_URL}/api/v1/accounts/{customer_id}/profile"
    print("url---", url, customer_id)
    async with httpx.AsyncClient() as client:
        response = await client.get(url)

    response.raise_for_status()

    return response.json()