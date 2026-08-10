package main

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dataSourceSchema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var _ datasource.DataSource = &RGWRoleDataSource{}

func newRGWRoleDataSource() datasource.DataSource {
	return &RGWRoleDataSource{}
}

type RGWRoleDataSource struct {
	client *restapi.Client
}

type RGWRoleDataSourceModel struct {
	Name                     types.String         `tfsdk:"name"`
	Path                     types.String         `tfsdk:"path"`
	AssumeRolePolicyDocument types.String         `tfsdk:"assume_role_policy_document"`
	MaxSessionDuration       types.Int64          `tfsdk:"max_session_duration"`
	ID                       types.String         `tfsdk:"id"`
	RoleID                   types.String         `tfsdk:"role_id"`
	Arn                      types.String         `tfsdk:"arn"`
	CreateDate               types.String         `tfsdk:"create_date"`
	AccountID                types.String         `tfsdk:"account_id"`
	Description              types.String         `tfsdk:"description"`
	PermissionPolicies       jsontypes.Normalized `tfsdk:"permission_policies"`
}

func (d *RGWRoleDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rgw_role"
}

func (d *RGWRoleDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dataSourceSchema.Schema{
		MarkdownDescription: "This data source allows you to get information about a Ceph RGW IAM role.",
		Attributes: map[string]dataSourceSchema.Attribute{
			"name": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the role.",
				Required:            true,
			},
			"path": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The path to the role.",
				Computed:            true,
			},
			"assume_role_policy_document": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The trust relationship policy document that grants an entity permission to assume the role.",
				Computed:            true,
			},
			"max_session_duration": dataSourceSchema.Int64Attribute{
				MarkdownDescription: "The maximum session duration in seconds for the role.",
				Computed:            true,
			},
			"id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The name of the role.",
				Computed:            true,
			},
			"role_id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The internal id of the role.",
				Computed:            true,
			},
			"arn": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The Amazon Resource Name (ARN) of the role.",
				Computed:            true,
			},
			"create_date": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The date and time the role was created.",
				Computed:            true,
			},
			"account_id": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The RGW account ID that scopes the role. Omit for legacy global roles on Squid and Tentacle 20.2.2; set for account-scoped roles on Tentacle 20.2.3 and later.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(rgwRoleAccountIDPattern, "must be RGW followed by 17 digits"),
				},
			},
			"description": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The description of the role. Not settable through the dashboard API.",
				Computed:            true,
			},
			"permission_policies": dataSourceSchema.StringAttribute{
				MarkdownDescription: "The permission policies attached to the role as a JSON list. Read-only; no dashboard endpoint manages them.",
				Computed:            true,
				CustomType:          jsontypes.NormalizedType{},
			},
		},
	}
}

func (d *RGWRoleDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*restapi.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *restapi.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.client = client
}

func (d *RGWRoleDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data RGWRoleDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	accountID := rgwRoleAccountID(data.AccountID)
	role, err := d.client.RGWGetRole(ctx, accountID, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"API Request Error",
			fmt.Sprintf("Unable to get RGW role from Ceph API: %s", err),
		)
		return
	}

	data.Name = types.StringValue(role.RoleName)
	data.Path = types.StringValue(role.Path)
	data.AssumeRolePolicyDocument = types.StringValue(role.AssumeRolePolicyDocument)
	data.MaxSessionDuration = types.Int64Value(role.MaxSessionDuration)
	data.ID = types.StringValue(role.RoleName)
	data.RoleID = types.StringValue(role.RoleID)
	data.Arn = types.StringValue(role.Arn)
	data.CreateDate = types.StringValue(role.CreateDate)
	if role.AccountID != "" || accountID == "" {
		data.AccountID = types.StringValue(role.AccountID)
	}
	data.Description = types.StringValue(role.Description)
	if policies := string(role.PermissionPolicies); policies != "" && policies != "null" {
		data.PermissionPolicies = jsontypes.NewNormalizedValue(policies)
	} else {
		data.PermissionPolicies = jsontypes.NewNormalizedValue("[]")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
