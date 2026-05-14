from pathlib import Path

import requests

API_URL = "http://24.199.123.7:1337/api"
API_TOKEN = "7cb7ab5ef29feb52a075ca15247a0cd22537e315e7f81cbc4b8de7aebb24d56db3cdd5d1f3fceb652f192e6fd8524913ea5efcc7a1b9b098fe504a0cd7be9e8259ee411a7fdd93f3526a52769a76538902e1a4890de5470a8f2810c69f20c3d54455ae5554ae8e39646559dbdf3461bc616bdf9eec1ae90391450a739a5c0871"


headers = {
    "Authorization": f"Bearer {API_TOKEN}",
    "Content-Type": "application/json"
}

# -------- API HELPERS -------- #
def create_entry(endpoint, data):
    url = f"{API_URL}/{endpoint}"
    res = requests.post(url, headers=headers, json={"data": data})
    print(res.text)
    res.raise_for_status()
    return res.json()["data"]["documentId"]

def update_entry(endpoint, entry_id, data):
    url = f"{API_URL}/{endpoint}/{entry_id}"
    res = requests.put(url, headers=headers, json={"data": data})
    res.raise_for_status()
    print(res.text)
    return res.json()["data"]["documentId"]


def upload_media(file_path, upload_path):
    path = Path(file_path)
    with path.open("rb") as file_obj:
        res = requests.post(
            f"{API_URL}/upload",
            headers={"Authorization": f"Bearer {API_TOKEN}"},
            files={"files": (path.name, file_obj)},
            data={"path": upload_path},
        )

    print(res.text)
    res.raise_for_status()
    payload = res.json()

    if isinstance(payload, list) and payload:
        return payload[0]["id"]

    if isinstance(payload, dict):
        data = payload.get("data")
        if isinstance(data, list) and data:
            return data[0]["id"]
        if isinstance(data, dict) and "id" in data:
            return data["id"]

    raise RuntimeError("Unexpected upload response from Strapi")


def attach_module_video(module_id, module, upload_path="app/weekly_module_videos"):
    video_ref = module.get("video")
    video_file = module.get("video_file")
    video_asset_id = module.get("video_asset_id")

    if isinstance(video_ref, dict):
        video_file = video_ref.get("file_path") or video_ref.get("path") or video_file
        video_asset_id = video_ref.get("asset_id") or video_ref.get("id") or video_asset_id
    elif isinstance(video_ref, (int, str)):
        video_asset_id = video_ref

    if video_file:
        video_asset_id = upload_media(video_file, upload_path)

    if video_asset_id is not None:
        update_entry("course-modules", module_id, {"video": video_asset_id})


def resolve_record_id(record, preferred_keys):
    if not isinstance(record, dict):
        return None

    for key in preferred_keys:
        value = record.get(key)
        if value:
            return value

    return None

# -------- SEEDING -------- #
def seed_weekly_modules(modules):
    for idx, module in enumerate(modules, start=1):
        print(f"Creating weekly module: {module['title']}")
        module_id = resolve_record_id(module, ["document_id", "module_document_id", "id"])
        if module_id is None:
            module_id = create_entry("course-modules", {
                "title": module["title"],
                "description": module["description"],
                "type": "weekly",
            })

        attach_module_video(module_id, module)

        quiz = module["quiz"]
        print(f"  Creating quiz: {quiz['title']}")
        quiz_id = create_entry("quizzes", {
            "title": quiz["title"],
            "quiz_type": quiz["quiz_type"],
            "format": quiz["format"],
            "passing_score": 70,
            "estimated_completion_time_in_mins": 20
        })

        # link quiz to module
        update_entry("course-modules", module_id, {"quiz": quiz_id})

        question_ids = []
        for question in quiz["questions"]:
            print(f"    Creating question: {question['prompt']}")
            q_id = create_entry("questions", {
                "prompt": question["prompt"],
                "question_type": question["question_type"],
                "explanation": question["explanation"],
                "quiz": quiz_id
            })
            question_ids.append(q_id)

            option_ids = []
            for option in question["answer_options"]:
                option_id = create_entry("answer-options", {
                    "text": option["text"],
                    "is_correct": option["is_correct"],
                    "questions": [q_id]
                })
                option_ids.append(option_id)

            # update question with options
            update_entry("questions", q_id, {"answer_options": option_ids})

        # update quiz with questions
        update_entry("quizzes", quiz_id, {"questions": question_ids})


# -------- WEEKLY MODULE DATA -------- #
weekly_modules = [
    {
        "title": "Weekly Tech Trends",
        "description": "Explore the latest trends in technology and how they affect your digital life.",
        "quiz": {
            "title": "Weekly Tech Trends Quiz",
            "quiz_type": "scored",
            "format": "true-false",
            "questions": [
                {
                    "prompt": "New apps should always be downloaded from trusted sources.",
                    "question_type": "true-false",
                    "explanation": "Untrusted apps may contain malware.",
                    "answer_options": [
                        {"text": "True", "is_correct": True},
                        {"text": "False", "is_correct": False}
                    ]
                },
                {
                    "prompt": "Which of these are current tech trends?",
                    "question_type": "multiple-choice",
                    "explanation": "AI, blockchain, and VR are key tech trends.",
                    "answer_options": [
                        {"text": "AI", "is_correct": True},
                        {"text": "Blockchain", "is_correct": True},
                        {"text": "Flip phones", "is_correct": False},
                        {"text": "Virtual Reality", "is_correct": True}
                    ]
                },
                {
                    "prompt": "What is AI short for?",
                    "question_type": "single-choice",
                    "explanation": "AI stands for Artificial Intelligence.",
                    "answer_options": [
                        {"text": "Artificial Imagination", "is_correct": False},
                        {"text": "Artificial Intelligence", "is_correct": True},
                        {"text": "Automatic Input", "is_correct": False},
                        {"text": "Advanced Internet", "is_correct": False}
                    ]
                }
            ]
        }
    },
    {
        "title": "Weekly Cyber Safety",
        "description": "Learn how to keep your accounts and personal information secure every week.",
        "quiz": {
            "title": "Weekly Cyber Safety Quiz",
            "quiz_type": "scored",
            "format": "true-false",
            "questions": [
                {
                    "prompt": "Two-factor authentication adds extra security.",
                    "question_type": "true-false",
                    "explanation": "2FA makes accounts harder to hack.",
                    "answer_options": [
                        {"text": "True", "is_correct": True},
                        {"text": "False", "is_correct": False}
                    ]
                },
                {
                    "prompt": "Which are good password practices?",
                    "question_type": "multiple-choice",
                    "explanation": "Long, unique, and mixed-character passwords are safe.",
                    "answer_options": [
                        {"text": "Using your name", "is_correct": False},
                        {"text": "12+ characters", "is_correct": True},
                        {"text": "Mixing numbers & symbols", "is_correct": True},
                        {"text": "Reusing passwords", "is_correct": False}
                    ]
                },
                {
                    "prompt": "What should you do if you suspect your account is hacked?",
                    "question_type": "single-choice",
                    "explanation": "Changing your password immediately helps.",
                    "answer_options": [
                        {"text": "Ignore it", "is_correct": False},
                        {"text": "Change your password immediately", "is_correct": True},
                        {"text": "Tell no one", "is_correct": False},
                        {"text": "Keep using the account", "is_correct": False}
                    ]
                }
            ]
        }
    },
    {
        "title": "Weekly Digital Balance",
        "description": "Find balance between online activities, learning, and personal health.",
        "quiz": {
            "title": "Weekly Digital Balance Quiz",
            "quiz_type": "scored",
            "format": "true-false",
            "questions": [
                {
                    "prompt": "Too much screen time can affect your sleep.",
                    "question_type": "true-false",
                    "explanation": "Blue light disrupts sleep cycles.",
                    "answer_options": [
                        {"text": "True", "is_correct": True},
                        {"text": "False", "is_correct": False}
                    ]
                },
                {
                    "prompt": "What are ways to balance digital life?",
                    "question_type": "multiple-choice",
                    "explanation": "Taking breaks, exercise, hobbies bring balance.",
                    "answer_options": [
                        {"text": "Taking breaks", "is_correct": True},
                        {"text": "Exercising", "is_correct": True},
                        {"text": "Ignoring friends", "is_correct": False},
                        {"text": "Doing hobbies", "is_correct": True}
                    ]
                },
                {
                    "prompt": "What’s one way to reduce screen time before bed?",
                    "question_type": "single-choice",
                    "explanation": "Reading a book helps relax without screens.",
                    "answer_options": [
                        {"text": "Playing video games", "is_correct": False},
                        {"text": "Reading a book", "is_correct": True},
                        {"text": "Watching TV", "is_correct": False},
                        {"text": "Scrolling social media", "is_correct": False}
                    ]
                }
            ]
        }
    }
]

# -------- RUN -------- #
if __name__ == "__main__":
    seed_weekly_modules(weekly_modules)
    print("✅ Weekly modules seeded successfully!")
