import requests

API_URL = "http://24.199.123.7:1337/api"
API_TOKEN = "7cb7ab5ef29feb52a075ca15247a0cd22537e315e7f81cbc4b8de7aebb24d56db3cdd5d1f3fceb652f192e6fd8524913ea5efcc7a1b9b098fe504a0cd7be9e8259ee411a7fdd93f3526a52769a76538902e1a4890de5470a8f2810c69f20c3d54455ae5554ae8e39646559dbdf3461bc616bdf9eec1ae90391450a739a5c0871"

headers = {"Authorization": f"Bearer {API_TOKEN}", "Content-Type": "application/json"}


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
    print(res.text)
    res.raise_for_status()
    return res.json()["data"]["documentId"]


# -------- SEEDING -------- #
def seed_courses(courses):
    for course in courses:
        print(f"Creating course: {course['title']}")
        course_id = create_entry(
            "courses", {"title": course["title"], "description": course["description"]}
        )

        module_ids = []
        for idx, module in enumerate(course["modules"], start=1):
            print(f"  Creating module: {module['title']}")
            module_id = create_entry(
                "course-modules",
                {
                    "title": module["title"],
                    "description": module["description"],
                    "type": "course",
                    "courses": [course_id],
                },
            )
            module_ids.append(module_id)

            quiz = module["quiz"]
            print(f"    Creating quiz: {quiz['title']}")
            quiz_id = create_entry(
                "quizzes",
                {
                    "title": quiz["title"],
                    "quiz_type": quiz["quiz_type"],
                    "format": quiz["format"],
                    "passing_score": 80,
                    "estimated_completion_time_in_mins": 30,
                },
            )

            # update module with quiz relation
            update_entry("course-modules", module_id, {"quiz": quiz_id})

            question_ids = []
            for question in quiz["questions"]:
                print(f"      Creating question: {question['prompt']}")
                q_id = create_entry(
                    "questions",
                    {
                        "prompt": question["prompt"],
                        "question_type": question["question_type"],
                        "explanation": question["explanation"],
                        "quiz": quiz_id,
                    },
                )
                question_ids.append(q_id)

                option_ids = []
                for option in question["answer_options"]:
                    option_id = create_entry(
                        "answer-options",
                        {
                            "text": option["text"],
                            "is_correct": option["is_correct"],
                            "questions": [q_id],
                        },
                    )
                    option_ids.append(option_id)

                # update question with options
                update_entry("questions", q_id, {"answer_options": option_ids})

            # update quiz with questions
            update_entry("quizzes", quiz_id, {"questions": question_ids})

        # update course with modules
        update_entry("courses", course_id, {"course_modules": module_ids})


# -------- FULL DIGITAL PERMIT CURRICULUM -------- #
courses = [
    # ---------------- Course 1 ---------------- #
    {
        "title": "Digital Responsibility Basics",
        "description": "Learn the foundations of digital citizenship—understanding devices, online behavior, and safe choices.",
        "modules": [
            {
                "title": "Understanding Your Device",
                "description": "Explore what personal devices are, how they work, and why using them responsibly matters.",
                "quiz": {
                    "title": "Understanding Your Device Quiz",
                    "quiz_type": "scored",
                    "format": "single-choice",
                    "questions": [
                        {
                            "prompt": "What is the main purpose of a personal device?",
                            "question_type": "single-choice",
                            "explanation": "Devices should be used to connect, learn, and communicate.",
                            "answer_options": [
                                {"text": "To play games only", "is_correct": False},
                                {
                                    "text": "To connect, learn, and communicate",
                                    "is_correct": True,
                                },
                                {"text": "To waste time", "is_correct": False},
                                {"text": "To avoid people", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "It’s safe to share your passwords with friends.",
                            "question_type": "true-false",
                            "explanation": "Passwords should never be shared.",
                            "answer_options": [
                                {"text": "True", "is_correct": False},
                                {"text": "False", "is_correct": True},
                            ],
                        },
                        {
                            "prompt": "Which of these should you keep private?",
                            "question_type": "multiple-choice",
                            "explanation": "Sensitive details like passwords, name, and address must be private.",
                            "answer_options": [
                                {"text": "Passwords", "is_correct": True},
                                {"text": "Full name", "is_correct": True},
                                {"text": "Favorite color", "is_correct": False},
                                {"text": "Home address", "is_correct": True},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Online Safety",
                "description": "Learn strategies to stay safe from strangers, scams, and unsafe online behavior.",
                "quiz": {
                    "title": "Online Safety Quiz",
                    "quiz_type": "scored",
                    "format": "multiple-choice",
                    "questions": [
                        {
                            "prompt": "What should you do if a stranger messages you online?",
                            "question_type": "single-choice",
                            "explanation": "Best response is to ignore, block, or tell a trusted adult.",
                            "answer_options": [
                                {"text": "Reply and make friends", "is_correct": False},
                                {
                                    "text": "Ignore, block, or tell a trusted adult",
                                    "is_correct": True,
                                },
                                {"text": "Share personal info", "is_correct": False},
                                {"text": "Accept their request", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Which of these are signs of a strong password?",
                            "question_type": "multiple-choice",
                            "explanation": "Strong passwords mix numbers, symbols, and letter cases.",
                            "answer_options": [
                                {"text": "Includes numbers", "is_correct": True},
                                {
                                    "text": "Includes special symbols",
                                    "is_correct": True,
                                },
                                {"text": "Only uses your name", "is_correct": False},
                                {
                                    "text": "Mix of upper & lower case",
                                    "is_correct": True,
                                },
                            ],
                        },
                        {
                            "prompt": "Clicking unknown links can be dangerous.",
                            "question_type": "true-false",
                            "explanation": "Unknown links may contain scams or viruses.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Digital Etiquette",
                "description": "Discover the importance of respect, kindness, and inclusion in online spaces.",
                "quiz": {
                    "title": "Digital Etiquette Quiz",
                    "quiz_type": "scored",
                    "format": "single-choice",
                    "questions": [
                        {
                            "prompt": "How should you behave in group chats?",
                            "question_type": "single-choice",
                            "explanation": "Respectful behavior includes kindness and including others.",
                            "answer_options": [
                                {"text": "Always spam memes", "is_correct": False},
                                {
                                    "text": "Be respectful and include others",
                                    "is_correct": True,
                                },
                                {"text": "Ignore everyone", "is_correct": False},
                                {"text": "Use only emojis", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Being kind online is just as important as being kind offline.",
                            "question_type": "true-false",
                            "explanation": "Online kindness is equally important.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Good online behavior includes:",
                            "question_type": "multiple-choice",
                            "explanation": "Respecting others and using polite language builds safe spaces.",
                            "answer_options": [
                                {"text": "Respecting others", "is_correct": True},
                                {"text": "Spreading rumors", "is_correct": False},
                                {"text": "Using polite language", "is_correct": True},
                                {"text": "Ignoring bullying", "is_correct": True},
                            ],
                        },
                    ],
                },
            },
        ],
    },
    # ---------------- Course 2 ---------------- #
    {
        "title": "Healthy Device Habits",
        "description": "Develop balanced habits to avoid screen addiction and ensure healthy tech use.",
        "modules": [
            {
                "title": "Screen Time Awareness",
                "description": "Learn how to balance fun, school, and rest by managing your screen time wisely.",
                "quiz": {
                    "title": "Screen Time Awareness Quiz",
                    "quiz_type": "scored",
                    "format": "single-choice",
                    "questions": [
                        {
                            "prompt": "What’s a healthy daily screen time goal for teens?",
                            "question_type": "single-choice",
                            "explanation": "Experts recommend around 1–2 hours of recreational screen time daily.",
                            "answer_options": [
                                {"text": "24 hours", "is_correct": False},
                                {"text": "About 1–2 hours", "is_correct": True},
                                {"text": "12 hours", "is_correct": False},
                                {"text": "No limit", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Devices should not be used right before sleep.",
                            "question_type": "true-false",
                            "explanation": "Using devices before sleep disrupts rest.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "What can you do if you can’t stop playing a game?",
                            "question_type": "multiple-choice",
                            "explanation": "Limiting time, asking for help, and balancing activities help.",
                            "answer_options": [
                                {"text": "Set time limits", "is_correct": True},
                                {"text": "Ask a parent for help", "is_correct": True},
                                {"text": "Skip meals", "is_correct": False},
                                {
                                    "text": "Balance with other activities",
                                    "is_correct": True,
                                },
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Focus and Productivity",
                "description": "Learn tools and habits that help you focus, stay productive, and avoid distractions.",
                "quiz": {
                    "title": "Focus and Productivity Quiz",
                    "quiz_type": "scored",
                    "format": "multiple-choice",
                    "questions": [
                        {
                            "prompt": "Which of these helps you focus better?",
                            "question_type": "multiple-choice",
                            "explanation": "Notifications off, breaks, and timers boost focus.",
                            "answer_options": [
                                {
                                    "text": "Turning off notifications",
                                    "is_correct": True,
                                },
                                {
                                    "text": "Doing homework with social media open",
                                    "is_correct": False,
                                },
                                {"text": "Taking breaks", "is_correct": True},
                                {"text": "Using a timer", "is_correct": True},
                            ],
                        },
                        {
                            "prompt": "Multitasking on your phone always makes you faster.",
                            "question_type": "true-false",
                            "explanation": "Multitasking usually reduces efficiency.",
                            "answer_options": [
                                {"text": "True", "is_correct": False},
                                {"text": "False", "is_correct": True},
                            ],
                        },
                        {
                            "prompt": "What’s one way to reduce distractions?",
                            "question_type": "single-choice",
                            "explanation": "Turning off alerts cuts distractions.",
                            "answer_options": [
                                {"text": "Turning off alerts", "is_correct": True},
                                {"text": "Leaving all apps open", "is_correct": False},
                                {"text": "Switching devices", "is_correct": False},
                                {"text": "None of the above", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Physical Health",
                "description": "Explore how device use affects your body and ways to stay healthy while online.",
                "quiz": {
                    "title": "Physical Health Quiz",
                    "quiz_type": "scored",
                    "format": "true-false",
                    "questions": [
                        {
                            "prompt": "Posture is important while using devices.",
                            "question_type": "true-false",
                            "explanation": "Good posture reduces body strain.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "What helps prevent eye strain?",
                            "question_type": "multiple-choice",
                            "explanation": "Follow the 20–20–20 rule, adjust brightness, sit properly.",
                            "answer_options": [
                                {
                                    "text": "Looking away every 20 minutes",
                                    "is_correct": True,
                                },
                                {
                                    "text": "Adjusting screen brightness",
                                    "is_correct": True,
                                },
                                {"text": "Never blinking", "is_correct": False},
                                {"text": "Sitting too close", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "If your neck hurts from scrolling, what should you do?",
                            "question_type": "single-choice",
                            "explanation": "Breaks and stretching reduce pain.",
                            "answer_options": [
                                {
                                    "text": "Take a break and stretch",
                                    "is_correct": True,
                                },
                                {"text": "Keep scrolling", "is_correct": False},
                                {"text": "Ignore the pain", "is_correct": False},
                                {"text": "Use your device in bed", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
        ],
    },
    # ---------------- Course 3 ---------------- #
    {
        "title": "Social Media Smarts",
        "description": "Learn how to navigate social media wisely, protect your identity, and engage positively.",
        "modules": [
            {
                "title": "Privacy Settings",
                "description": "Understand privacy tools to keep your identity and personal details safe online.",
                "quiz": {
                    "title": "Privacy Settings Quiz",
                    "quiz_type": "scored",
                    "format": "single-choice",
                    "questions": [
                        {
                            "prompt": "Why should accounts be set to private?",
                            "question_type": "single-choice",
                            "explanation": "Private accounts let you control who sees your posts.",
                            "answer_options": [
                                {
                                    "text": "To control who sees your posts",
                                    "is_correct": True,
                                },
                                {"text": "To get more likes", "is_correct": False},
                                {
                                    "text": "To avoid responsibility",
                                    "is_correct": False,
                                },
                                {"text": "To hide from friends", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Never share your password online.",
                            "question_type": "true-false",
                            "explanation": "Passwords should stay private always.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Which info should you avoid sharing publicly?",
                            "question_type": "multiple-choice",
                            "explanation": "Birthdays, addresses, and schools risk safety.",
                            "answer_options": [
                                {"text": "Birthday", "is_correct": True},
                                {"text": "School name", "is_correct": True},
                                {"text": "Favorite TV show", "is_correct": False},
                                {"text": "Address", "is_correct": True},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Digital Identity",
                "description": "Learn how your posts, likes, and shares shape the digital version of yourself.",
                "quiz": {
                    "title": "Digital Identity Quiz",
                    "quiz_type": "scored",
                    "format": "true-false",
                    "questions": [
                        {
                            "prompt": "Deleted posts are gone forever.",
                            "question_type": "true-false",
                            "explanation": "Deleted posts can often still be recovered or screenshotted.",
                            "answer_options": [
                                {"text": "True", "is_correct": False},
                                {"text": "False", "is_correct": True},
                            ],
                        },
                        {
                            "prompt": "Which of these is safe to post online?",
                            "question_type": "single-choice",
                            "explanation": "Pet pictures are safe. Avoid private details.",
                            "answer_options": [
                                {"text": "Home address", "is_correct": False},
                                {"text": "A funny pet picture", "is_correct": True},
                                {"text": "Phone number", "is_correct": False},
                                {"text": "Report card", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Your digital identity includes:",
                            "question_type": "multiple-choice",
                            "explanation": "Photos, comments, likes, and shares define identity.",
                            "answer_options": [
                                {"text": "Photos", "is_correct": True},
                                {"text": "Comments", "is_correct": True},
                                {"text": "Passwords", "is_correct": False},
                                {"text": "Likes and shares", "is_correct": True},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Online Kindness",
                "description": "Discover why kindness matters and how to stand against cyberbullying.",
                "quiz": {
                    "title": "Online Kindness Quiz",
                    "quiz_type": "scored",
                    "format": "single-choice",
                    "questions": [
                        {
                            "prompt": "What should you do if you see cyberbullying?",
                            "question_type": "single-choice",
                            "explanation": "Reporting or helping the victim is best.",
                            "answer_options": [
                                {"text": "Join in", "is_correct": False},
                                {
                                    "text": "Report or support the victim",
                                    "is_correct": True,
                                },
                                {"text": "Ignore", "is_correct": False},
                                {"text": "Laugh", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Online jokes can sometimes hurt people.",
                            "question_type": "true-false",
                            "explanation": "Jokes can harm feelings when spread online.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Ways to spread kindness include:",
                            "question_type": "multiple-choice",
                            "explanation": "Kind comments, encouragement, support show kindness.",
                            "answer_options": [
                                {"text": "Positive comments", "is_correct": True},
                                {"text": "Encouraging friends", "is_correct": True},
                                {"text": "Spamming memes", "is_correct": False},
                                {"text": "Supporting classmates", "is_correct": True},
                            ],
                        },
                    ],
                },
            },
        ],
    },
    # ---------------- Course 4 ---------------- #
    {
        "title": "Critical Thinking Online",
        "description": "Develop the skills to tell real from fake online and think before you click.",
        "modules": [
            {
                "title": "Spotting Misinformation",
                "description": "Learn how to detect fake news, scams, and misleading information online.",
                "quiz": {
                    "title": "Spotting Misinformation Quiz",
                    "quiz_type": "scored",
                    "format": "true-false",
                    "questions": [
                        {
                            "prompt": "If a headline seems too crazy to be true, it probably is.",
                            "question_type": "true-false",
                            "explanation": "Wild headlines are often clickbait or fake.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Which is a reliable news source?",
                            "question_type": "single-choice",
                            "explanation": "Trusted outlets cross-check facts.",
                            "answer_options": [
                                {"text": "A random blog", "is_correct": False},
                                {"text": "A trusted news outlet", "is_correct": True},
                                {"text": "A viral meme", "is_correct": False},
                                {"text": "Friend’s rumor", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Ways to verify info include:",
                            "question_type": "multiple-choice",
                            "explanation": "Fact-checking, comparing sites, checking dates helps.",
                            "answer_options": [
                                {"text": "Fact-checking websites", "is_correct": True},
                                {
                                    "text": "Cross-checking with other sites",
                                    "is_correct": True,
                                },
                                {"text": "Checking dates", "is_correct": True},
                                {"text": "Sharing instantly", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Thinking Before You Click",
                "description": "Explore why you should pause and analyze before clicking links or sharing content.",
                "quiz": {
                    "title": "Thinking Before You Click Quiz",
                    "quiz_type": "scored",
                    "format": "true-false",
                    "questions": [
                        {
                            "prompt": "Clicking unknown links is always safe.",
                            "question_type": "true-false",
                            "explanation": "Unknown links may lead to scams.",
                            "answer_options": [
                                {"text": "True", "is_correct": False},
                                {"text": "False", "is_correct": True},
                            ],
                        },
                        {
                            "prompt": "Which should you do before clicking a link?",
                            "question_type": "single-choice",
                            "explanation": "Hovering over the link helps check where it goes.",
                            "answer_options": [
                                {"text": "Click right away", "is_correct": False},
                                {"text": "Hover to preview URL", "is_correct": True},
                                {"text": "Ignore warnings", "is_correct": False},
                                {"text": "Share link first", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Safe clicking includes:",
                            "question_type": "multiple-choice",
                            "explanation": "Checking URLs, avoiding popups, antivirus helps.",
                            "answer_options": [
                                {"text": "Checking URLs", "is_correct": True},
                                {"text": "Avoiding popups", "is_correct": True},
                                {"text": "Installing antivirus", "is_correct": True},
                                {"text": "Clicking everything", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Evaluating Online Sources",
                "description": "Learn to assess if a source is trustworthy, relevant, and accurate.",
                "quiz": {
                    "title": "Evaluating Online Sources Quiz",
                    "quiz_type": "scored",
                    "format": "true-false",
                    "questions": [
                        {
                            "prompt": "Trustworthy sources always list their authors.",
                            "question_type": "true-false",
                            "explanation": "Reliable sources show authors and credentials.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Which is most reliable?",
                            "question_type": "single-choice",
                            "explanation": "Official org sites (.gov, .edu) are reliable.",
                            "answer_options": [
                                {"text": "Random tweet", "is_correct": False},
                                {"text": "Official gov website", "is_correct": True},
                                {"text": "Funny meme", "is_correct": False},
                                {"text": "Unverified forum", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "What should you check before trusting a site?",
                            "question_type": "multiple-choice",
                            "explanation": "Check author, date, references, site credibility.",
                            "answer_options": [
                                {"text": "Author credentials", "is_correct": True},
                                {"text": "Publication date", "is_correct": True},
                                {"text": "References", "is_correct": True},
                                {"text": "If friends shared it", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
        ],
    },
    # ---------------- Course 5 ---------------- #
    {
        "title": "Digital Citizenship in Action",
        "description": "Put your digital skills into practice to be a leader and responsible online citizen.",
        "modules": [
            {
                "title": "Being an Upstander",
                "description": "Learn how to take positive action when you see harmful online behavior.",
                "quiz": {
                    "title": "Being an Upstander Quiz",
                    "quiz_type": "scored",
                    "format": "single-choice",
                    "questions": [
                        {
                            "prompt": "What is an upstander?",
                            "question_type": "single-choice",
                            "explanation": "An upstander helps stop harmful behavior.",
                            "answer_options": [
                                {
                                    "text": "Someone who ignores bullying",
                                    "is_correct": False,
                                },
                                {
                                    "text": "Someone who stands up against harm",
                                    "is_correct": True,
                                },
                                {
                                    "text": "Someone who spreads rumors",
                                    "is_correct": False,
                                },
                                {
                                    "text": "Someone who blocks friends",
                                    "is_correct": False,
                                },
                            ],
                        },
                        {
                            "prompt": "Upstanders make the internet safer.",
                            "question_type": "true-false",
                            "explanation": "Standing up improves safety online.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "How can you be an upstander?",
                            "question_type": "multiple-choice",
                            "explanation": "Report, support, speak up are upstander actions.",
                            "answer_options": [
                                {"text": "Reporting bad behavior", "is_correct": True},
                                {"text": "Supporting victims", "is_correct": True},
                                {"text": "Spreading kindness", "is_correct": True},
                                {"text": "Laughing at bullying", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Digital Footprint",
                "description": "Understand how your online actions leave a permanent record.",
                "quiz": {
                    "title": "Digital Footprint Quiz",
                    "quiz_type": "scored",
                    "format": "true-false",
                    "questions": [
                        {
                            "prompt": "Your digital footprint disappears over time.",
                            "question_type": "true-false",
                            "explanation": "Digital footprints are long-lasting.",
                            "answer_options": [
                                {"text": "True", "is_correct": False},
                                {"text": "False", "is_correct": True},
                            ],
                        },
                        {
                            "prompt": "What creates a digital footprint?",
                            "question_type": "multiple-choice",
                            "explanation": "Posts, comments, likes all leave footprints.",
                            "answer_options": [
                                {"text": "Posts", "is_correct": True},
                                {"text": "Comments", "is_correct": True},
                                {"text": "Likes", "is_correct": True},
                                {"text": "Thinking privately", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Which is safest to post long-term?",
                            "question_type": "single-choice",
                            "explanation": "Positive achievements are safe.",
                            "answer_options": [
                                {"text": "Your full home address", "is_correct": False},
                                {"text": "A positive achievement", "is_correct": True},
                                {"text": "Passwords", "is_correct": False},
                                {"text": "Angry rants", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
            {
                "title": "Digital Leadership",
                "description": "Lead by example to inspire others toward responsible digital behavior.",
                "quiz": {
                    "title": "Digital Leadership Quiz",
                    "quiz_type": "scored",
                    "format": "true-false",
                    "questions": [
                        {
                            "prompt": "Leaders inspire positive online spaces.",
                            "question_type": "true-false",
                            "explanation": "Leaders model positive digital behavior.",
                            "answer_options": [
                                {"text": "True", "is_correct": True},
                                {"text": "False", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "Which is a sign of digital leadership?",
                            "question_type": "multiple-choice",
                            "explanation": "Encouraging, teaching, creating safe spaces are leadership.",
                            "answer_options": [
                                {"text": "Encouraging peers", "is_correct": True},
                                {"text": "Teaching others", "is_correct": True},
                                {"text": "Creating safe spaces", "is_correct": True},
                                {"text": "Bullying others", "is_correct": False},
                            ],
                        },
                        {
                            "prompt": "What’s one way to lead online?",
                            "question_type": "single-choice",
                            "explanation": "Sharing helpful resources is leadership.",
                            "answer_options": [
                                {
                                    "text": "Sharing helpful resources",
                                    "is_correct": True,
                                },
                                {"text": "Spamming chats", "is_correct": False},
                                {"text": "Ignoring kindness", "is_correct": False},
                                {"text": "Starting fights", "is_correct": False},
                            ],
                        },
                    ],
                },
            },
        ],
    },
]

# -------- RUN -------- #
if __name__ == "__main__":
    seed_courses(courses)
    print("✅ Seeding complete with relations!")
