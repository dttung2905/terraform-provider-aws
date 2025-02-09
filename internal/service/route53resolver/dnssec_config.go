package route53resolver

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/route53resolver"
	"github.com/hashicorp/aws-sdk-go-base/v2/awsv1shim/v2/tfawserr"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/tfresource"
)

// @SDKResource("aws_route53_resolver_dnssec_config")
func ResourceDNSSECConfig() *schema.Resource {
	return &schema.Resource{
		CreateWithoutTimeout: resourceDNSSECConfigCreate,
		ReadWithoutTimeout:   resourceDNSSECConfigRead,
		DeleteWithoutTimeout: resourceDNSSECConfigDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},

		Schema: map[string]*schema.Schema{
			"arn": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"owner_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"resource_id": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"validation_status": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceDNSSECConfigCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).Route53ResolverConn()

	input := &route53resolver.UpdateResolverDnssecConfigInput{
		ResourceId: aws.String(d.Get("resource_id").(string)),
		Validation: aws.String(route53resolver.ValidationEnable),
	}

	output, err := conn.UpdateResolverDnssecConfigWithContext(ctx, input)

	if err != nil {
		return diag.Errorf("creating Route53 Resolver DNSSEC Config: %s", err)
	}

	d.SetId(aws.StringValue(output.ResolverDNSSECConfig.Id))

	if _, err := waitDNSSECConfigCreated(ctx, conn, d.Id()); err != nil {
		return diag.Errorf("waiting for Route53 Resolver DNSSEC Config (%s) create: %s", d.Id(), err)
	}

	return resourceDNSSECConfigRead(ctx, d, meta)
}

func resourceDNSSECConfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).Route53ResolverConn()

	dnssecConfig, err := FindResolverDNSSECConfigByID(ctx, conn, d.Id())

	if !d.IsNewResource() && tfresource.NotFound(err) {
		log.Printf("[WARN] Route53 Resolver DNSSEC Config (%s) not found, removing from state", d.Id())
		d.SetId("")
		return nil
	}

	if err != nil {
		return diag.Errorf("reading Route53 Resolver DNSSEC Config (%s): %s", d.Id(), err)
	}

	ownerID := aws.StringValue(dnssecConfig.OwnerId)
	resourceID := aws.StringValue(dnssecConfig.ResourceId)
	arn := arn.ARN{
		Partition: meta.(*conns.AWSClient).Partition,
		Service:   "route53resolver",
		Region:    meta.(*conns.AWSClient).Region,
		AccountID: ownerID,
		Resource:  fmt.Sprintf("resolver-dnssec-config/%s", resourceID),
	}.String()
	d.Set("arn", arn)
	d.Set("owner_id", ownerID)
	d.Set("resource_id", resourceID)
	d.Set("validation_status", dnssecConfig.ValidationStatus)

	return nil
}

func resourceDNSSECConfigDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	conn := meta.(*conns.AWSClient).Route53ResolverConn()

	log.Printf("[DEBUG] Deleting Route53 Resolver DNSSEC Config: %s", d.Id())
	_, err := conn.UpdateResolverDnssecConfigWithContext(ctx, &route53resolver.UpdateResolverDnssecConfigInput{
		ResourceId: aws.String(d.Get("resource_id").(string)),
		Validation: aws.String(route53resolver.ValidationDisable),
	})

	if tfawserr.ErrCodeEquals(err, route53resolver.ErrCodeAccessDeniedException) {
		// VPC doesn't exist.
		return nil
	}

	if err != nil {
		return diag.Errorf("deleting Route53 Resolver DNSSEC Config (%s): %s", d.Id(), err)
	}

	if _, err = waitDNSSECConfigDeleted(ctx, conn, d.Id()); err != nil {
		return diag.Errorf("waiting for Route53 Resolver DNSSEC Config (%s) delete: %s", d.Id(), err)
	}

	return nil
}

func FindResolverDNSSECConfigByID(ctx context.Context, conn *route53resolver.Route53Resolver, id string) (*route53resolver.ResolverDnssecConfig, error) {
	input := &route53resolver.ListResolverDnssecConfigsInput{}
	var output *route53resolver.ResolverDnssecConfig

	// GetResolverDnssecConfig does not support query by ID.
	err := conn.ListResolverDnssecConfigsPagesWithContext(ctx, input, func(page *route53resolver.ListResolverDnssecConfigsOutput, lastPage bool) bool {
		if page == nil {
			return !lastPage
		}

		for _, v := range page.ResolverDnssecConfigs {
			if aws.StringValue(v.Id) == id {
				output = v

				return false
			}
		}

		return !lastPage
	})

	if err != nil {
		return nil, err
	}

	if output == nil {
		return nil, &resource.NotFoundError{LastRequest: input}
	}

	if validationStatus := aws.StringValue(output.ValidationStatus); validationStatus == route53resolver.ResolverDNSSECValidationStatusDisabled {
		return nil, &resource.NotFoundError{
			Message:     validationStatus,
			LastRequest: input,
		}
	}

	return output, nil
}

func statusDNSSECConfig(ctx context.Context, conn *route53resolver.Route53Resolver, id string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		output, err := FindResolverDNSSECConfigByID(ctx, conn, id)

		if tfresource.NotFound(err) {
			return nil, "", nil
		}

		if err != nil {
			return nil, "", err
		}

		return output, aws.StringValue(output.ValidationStatus), nil
	}
}

const (
	dnssecConfigCreatedTimeout = 10 * time.Minute
	dnssecConfigDeletedTimeout = 10 * time.Minute
)

func waitDNSSECConfigCreated(ctx context.Context, conn *route53resolver.Route53Resolver, id string) (*route53resolver.ResolverDnssecConfig, error) {
	stateConf := &resource.StateChangeConf{
		Pending: []string{route53resolver.ResolverDNSSECValidationStatusEnabling},
		Target:  []string{route53resolver.ResolverDNSSECValidationStatusEnabled},
		Refresh: statusDNSSECConfig(ctx, conn, id),
		Timeout: dnssecConfigCreatedTimeout,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*route53resolver.ResolverDnssecConfig); ok {
		return output, err
	}

	return nil, err
}

func waitDNSSECConfigDeleted(ctx context.Context, conn *route53resolver.Route53Resolver, id string) (*route53resolver.ResolverDnssecConfig, error) {
	stateConf := &resource.StateChangeConf{
		Pending: []string{route53resolver.ResolverDNSSECValidationStatusDisabling},
		Target:  []string{},
		Refresh: statusDNSSECConfig(ctx, conn, id),
		Timeout: dnssecConfigDeletedTimeout,
	}

	outputRaw, err := stateConf.WaitForStateContext(ctx)

	if output, ok := outputRaw.(*route53resolver.ResolverDnssecConfig); ok {
		return output, err
	}

	return nil, err
}
