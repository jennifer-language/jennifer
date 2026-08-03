# SPDX-License-Identifier: LGPL-3.0-only
# SPDX-FileCopyrightText: Copyright (C) 2026 mplx <jennifer@mplx.dev>

# orm_blog_demo.j - an end-to-end orm + sqlmigrate walkthrough over a small blog
# domain (authors, posts, tags with a many-to-many). It migrates the schema,
# seeds with insertMany / upsert, and eager-loads to show the fixed 1 + R query
# count. Point it at a throwaway database and run:
#
#     ORM_TEST_DRIVER=postgres \
#     ORM_TEST_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' \
#     jennifer run examples/modules/orm_blog_demo.j
#
# With no DSN set it just prints these instructions (so it is safe to run bare).

use io;
use os;
use sql;
import "../../modules/orm.j" as orm;
import "../../modules/sqlmigrate.j" as mig;

func dialectFor(driver as string) {
    if ($driver == "mysql" or $driver == "mariadb") {
        return orm.Dialect.Mysql;
    }
    return orm.Dialect.Postgres;
}

# addRow is a small helper to build a record and insert it.
func rec2(k1 as string, v1 as string, k2 as string, v2 as string) {
    def r as map of string to string init {};
    $r[$k1] = $v1;
    $r[$k2] = $v2;
    return $r;
}

func run(driver as string, dsn as string) {
    def dia as orm.Dialect init dialectFor($driver);
    def conn as sql.Connection init sql.open($driver, $dsn);
    defer sql.close($conn);
    def sess as orm.Session init orm.session($conn);

    # Schemas. Explicit integer keys keep the seed relationships simple.
    def authors as orm.Schema init orm.hasMany(
        orm.column(orm.column(orm.schema("authors", "id", $dia), "id", orm.ColumnKind.Int),
            "name", orm.ColumnKind.String),
        "posts", "posts", "authorId");
    def posts as orm.Schema init orm.manyToMany(
        orm.column(orm.column(orm.column(orm.schema("posts", "id", $dia), "id", orm.ColumnKind.Int),
            "authorId", orm.ColumnKind.Int), "title", orm.ColumnKind.String),
        "tags", "tags", "post_tags", "postId", "tagId");
    def tags as orm.Schema init orm.column(
        orm.column(orm.schema("tags", "id", $dia), "id", orm.ColumnKind.Int), "name", orm.ColumnKind.String);

    # Migrate: a single migration that creates the four tables (the join table's
    # composite key is hand-written DDL - orm.createTable emits a single-column PK).
    def m1 as mig.Migration init mig.Migration{
        version: "001",
        description: "blog schema",
        up: [
            orm.createTable($authors),
            orm.createTable($posts),
            orm.createTable($tags),
            "CREATE TABLE post_tags (postId INTEGER, tagId INTEGER, PRIMARY KEY (postId, tagId))"
        ],
        down: [
            "DROP TABLE post_tags",
            orm.dropTable("tags"),
            orm.dropTable("posts"),
            orm.dropTable("authors")
        ]
    };
    # Start clean, then apply.
    mig.rollbackMigrations($conn, [$m1], 1);
    io.printf("migrated %d\n", mig.migrate($conn, [$m1]));

    # Seed. insertMany writes the authors in one INSERT; upsert is idempotent.
    orm.insertMany($sess, $authors, [rec2("id", "1", "name", "ada"), rec2("id", "2", "name", "bob")]);
    orm.upsert($sess, $authors, rec2("id", "1", "name", "Ada Lovelace"), ["id"]);
    orm.insertMany($sess, $posts, [
        rec3("id", "10", "authorId", "1", "title", "hello"),
        rec3("id", "11", "authorId", "1", "title", "world"),
        rec3("id", "12", "authorId", "2", "title", "hi")
    ]);
    orm.insertMany($sess, $tags, [rec2("id", "100", "name", "go"), rec2("id", "101", "name", "sql")]);
    def pt as list of map of string to string init [
        rec2("postId", "10", "tagId", "100"),
        rec2("postId", "10", "tagId", "101"),
        rec2("postId", "11", "tagId", "100")
    ];
    for (def link in $pt) {
        sql.exec($conn, "INSERT INTO post_tags (postId, tagId) VALUES (" + $link["postId"] + ", " +
            $link["tagId"] + ")");
    }

    # Eager load: authors + their posts in 2 queries (authors, then posts IN ...).
    def byAuthor as orm.Result init orm.load($sess, $authors,
        orm.with(orm.orderBy(orm.from($authors), "id", "asc"), "posts"));
    for (def author in orm.rows($byAuthor)) {
        io.printf("%s: %d post(s)\n", $author["name"],
            len(orm.related($byAuthor, $author, "posts")));
    }

    # Many-to-many: each post's tags in 2 queries (posts, then the join query).
    def byPost as orm.Result init orm.load($sess, $posts,
        orm.with(orm.orderBy(orm.from($posts), "id", "asc"), "tags"));
    for (def post in orm.rows($byPost)) {
        def names as list of string init [];
        for (def t in orm.related($byPost, $post, "tags")) {
            $names[] = $t["name"];
        }
        io.printf("post %s tags: %d\n", $post["title"], len($names));
    }

    mig.rollbackMigrations($conn, [$m1], 1); # clean up
}

# rec3 builds a three-key record.
func rec3(k1 as string, v1 as string, k2 as string, v2 as string, k3 as string, v3 as string) {
    def r as map of string to string init {};
    $r[$k1] = $v1;
    $r[$k2] = $v2;
    $r[$k3] = $v3;
    return $r;
}

def driver as string init os.getEnv("ORM_TEST_DRIVER");
def dsn as string init os.getEnv("ORM_TEST_DSN");

if ($dsn == "" or $driver == "") {
    io.printf("orm_blog_demo: set ORM_TEST_DRIVER (postgres / mysql) and ORM_TEST_DSN to run live.\n");
    io.printf("it migrates authors / posts / tags (many-to-many), seeds via insertMany / upsert,\n");
    io.printf("and eager-loads authors->posts and posts->tags in a fixed 2 queries each.\n");
} else {
    run($driver, $dsn);
}
